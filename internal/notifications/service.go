package notifications

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

const (
	maxEndpointRunes  = 2048
	maxKeyRunes       = 200
	maxUserAgentRunes = 500
	maxTitleRunes     = 120
	maxBodyRunes      = 500
	maxURLRunes       = 500

	sendTimeout = 10 * time.Second
)

// Service owns subscriptions and push delivery (F9). It is inert (503 on
// writes) until IFTER_WEBPUSH_ENABLED is true and the VAPID triple is valid.
type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	log     *slog.Logger
	cfg     Config
	sender  Sender
}

// ConfigFromEnvironment reads the documented IFTER_WEBPUSH_* keys. An
// incomplete or malformed triple leaves the feature disabled instead of
// failing startup; the config endpoint then reports enabled=false.
func ConfigFromEnvironment() Config {
	cfg := Config{
		Enabled:    parseBool(os.Getenv("IFTER_WEBPUSH_ENABLED")),
		PublicKey:  strings.TrimSpace(os.Getenv("IFTER_WEBPUSH_PUBLIC_KEY")),
		PrivateKey: strings.TrimSpace(os.Getenv("IFTER_WEBPUSH_PRIVATE_KEY")),
		Subject:    strings.TrimSpace(os.Getenv("IFTER_WEBPUSH_SUBJECT")),
	}
	return cfg
}

func parseBool(value string) bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && enabled
}

func NewService(pool *pgxpool.Pool, log *slog.Logger, cfg Config) *Service {
	return NewServiceWithSender(pool, log, cfg, newWebPushSender(cfg))
}

func NewServiceWithSender(pool *pgxpool.Pool, log *slog.Logger, cfg Config, sender Sender) *Service {
	if log == nil {
		log = slog.Default()
	}
	service := &Service{
		pool:    pool,
		queries: db.New(pool),
		log:     log,
		cfg:     cfg,
		sender:  sender,
	}
	if !cfg.Enabled {
		return service
	}
	if problem := validateConfig(cfg); problem != "" {
		log.Warn("webpush_config_incomplete", "reason", problem)
		service.cfg.Enabled = false
	}
	return service
}

// validateConfig reports the first problem with an enabled VAPID triple, or "".
func validateConfig(cfg Config) string {
	if cfg.PublicKey == "" || cfg.PrivateKey == "" {
		return "missing_vapid_keys"
	}
	if !validBase64URL(cfg.PublicKey) || !validBase64URL(cfg.PrivateKey) {
		return "invalid_vapid_keys"
	}
	if cfg.Subject == "" {
		return "missing_subject"
	}
	if !strings.HasPrefix(cfg.Subject, "mailto:") && !strings.HasPrefix(cfg.Subject, "https://") {
		return "invalid_subject"
	}
	return ""
}

func validBase64URL(value string) bool {
	if _, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return true
	}
	_, err := base64.URLEncoding.DecodeString(value)
	return err == nil
}

// Config exposes GET /push/config. The public key is blank while disabled so
// the frontend never asks for notification permission.
func (s *Service) Config() ConfigView {
	if !s.cfg.Enabled {
		return ConfigView{Enabled: false, PublicKey: ""}
	}
	return ConfigView{Enabled: true, PublicKey: s.cfg.PublicKey}
}

func (s *Service) List(ctx context.Context, actor Actor) ([]Subscription, error) {
	userID, err := parseActor(actor)
	if err != nil {
		return nil, err
	}
	rows, err := s.queries.ListPushSubscriptionsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	items := make([]Subscription, 0, len(rows))
	for _, row := range rows {
		items = append(items, subscriptionFromModel(row))
	}
	return items, nil
}

// Subscribe upserts by endpoint: re-subscribing from the same browser
// refreshes the keys and moves ownership to the signed-in user.
func (s *Service) Subscribe(ctx context.Context, actor Actor, in SubscriptionInput) (Subscription, error) {
	userID, err := parseActor(actor)
	if err != nil {
		return Subscription{}, err
	}
	if !s.cfg.Enabled {
		return Subscription{}, ErrDisabled
	}
	in, err = normalizeInput(in)
	if err != nil {
		return Subscription{}, err
	}
	row, err := s.queries.UpsertPushSubscription(ctx, db.UpsertPushSubscriptionParams{
		UserID:    userID,
		Endpoint:  in.Endpoint,
		P256dh:    in.P256DH,
		Auth:      in.Auth,
		UserAgent: optionalText(in.UserAgent),
	})
	if err != nil {
		return Subscription{}, mapWriteError(fmt.Errorf("upsert push subscription: %w", err))
	}
	return subscriptionFromModel(row), nil
}

func (s *Service) Unsubscribe(ctx context.Context, actor Actor, idValue string) error {
	userID, err := parseActor(actor)
	if err != nil {
		return err
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return ErrInvalidInput
	}
	affected, err := s.queries.DeletePushSubscriptionForUser(ctx, db.DeletePushSubscriptionForUserParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// SendTest delivers a payload to the calling admin's own subscriptions
// (POST /admin/push/test).
func (s *Service) SendTest(ctx context.Context, actor Actor, in SendInput) (SendResult, error) {
	userID, err := parseActor(actor)
	if err != nil {
		return SendResult{}, err
	}
	if !hasRole(actor.Roles, "admin") {
		return SendResult{}, ErrForbidden
	}
	if !s.cfg.Enabled {
		return SendResult{}, ErrDisabled
	}
	payload, err := normalizePayload(Payload{Title: in.Title, Body: in.Body})
	if err != nil {
		return SendResult{}, err
	}
	rows, err := s.queries.ListPushSubscriptionsByUser(ctx, userID)
	if err != nil {
		return SendResult{}, fmt.Errorf("list push subscriptions: %w", err)
	}
	targets := make([]target, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, targetFromModel(row))
	}
	return s.send(ctx, targets, payload), nil
}

// NotifyRoles pushes to every subscriber holding one of the given roles (the
// discipline hook targets admin + principal). Delivery failures are counted,
// never returned, so they cannot break the triggering write.
func (s *Service) NotifyRoles(ctx context.Context, roles []string, payload Payload) (SendResult, error) {
	if !s.cfg.Enabled {
		return SendResult{}, nil
	}
	payload, err := normalizePayload(payload)
	if err != nil {
		return SendResult{}, err
	}
	rows, err := s.queries.ListPushSubscriptionsForRoles(ctx, roles)
	if err != nil {
		return SendResult{}, fmt.Errorf("list push subscriptions for roles: %w", err)
	}
	targets := make([]target, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, target{
			id:   row.ID,
			keys: SubscriptionKeys{Endpoint: row.Endpoint, P256DH: row.P256dh, Auth: row.Auth},
		})
	}
	return s.send(ctx, targets, payload), nil
}

type target struct {
	id   pgtype.UUID
	keys SubscriptionKeys
}

func targetFromModel(row db.PushSubscription) target {
	return target{
		id:   row.ID,
		keys: SubscriptionKeys{Endpoint: row.Endpoint, P256DH: row.P256dh, Auth: row.Auth},
	}
}

func (s *Service) send(ctx context.Context, targets []target, payload Payload) SendResult {
	if len(targets) == 0 {
		return SendResult{}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		s.log.Error("webpush_payload_encode_failed", "error", err)
		return SendResult{Failed: len(targets)}
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	var result SendResult
	for _, item := range targets {
		status, err := s.sender.Send(ctx, item.keys, data)
		if err == nil {
			result.Sent++
			continue
		}
		result.Failed++
		s.log.Warn("webpush_send_failed", "subscription_id", uuidString(item.id), "status", status, "error", err)
		// The push service permanently rejected this endpoint (unsubscribed
		// browser); drop the row so the next send does not repeat the failure.
		if status == http.StatusNotFound || status == http.StatusGone {
			if deleteErr := s.queries.DeletePushSubscription(context.WithoutCancel(ctx), item.id); deleteErr != nil {
				s.log.Warn("webpush_subscription_cleanup_failed", "subscription_id", uuidString(item.id), "error", deleteErr)
			}
		}
	}
	return result
}

func normalizePayload(payload Payload) (Payload, error) {
	title, err := cleanText(payload.Title, maxTitleRunes)
	if err != nil || title == "" {
		return Payload{}, ErrInvalidInput
	}
	body, err := cleanText(payload.Body, maxBodyRunes)
	if err != nil || body == "" {
		return Payload{}, ErrInvalidInput
	}
	url, err := cleanText(payload.URL, maxURLRunes)
	if err != nil {
		return Payload{}, ErrInvalidInput
	}
	return Payload{Title: title, Body: body, URL: url}, nil
}

// normalizeInput validates the browser-supplied subscription. The endpoint is
// a URL the server will POST to, so only public HTTPS hosts are accepted
// (prevents using push subscriptions as an SSRF primitive).
func normalizeInput(in SubscriptionInput) (SubscriptionInput, error) {
	endpoint := strings.TrimSpace(in.Endpoint)
	if endpoint == "" || utf8.RuneCountInString(endpoint) > maxEndpointRunes {
		return SubscriptionInput{}, ErrInvalidInput
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return SubscriptionInput{}, ErrInvalidInput
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil &&
		(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()) {
		return SubscriptionInput{}, ErrInvalidInput
	}
	p256dh := strings.TrimSpace(in.P256DH)
	auth := strings.TrimSpace(in.Auth)
	if !validSubscriptionKey(p256dh) || !validSubscriptionKey(auth) {
		return SubscriptionInput{}, ErrInvalidInput
	}
	userAgent, err := cleanText(in.UserAgent, maxUserAgentRunes)
	if err != nil {
		return SubscriptionInput{}, ErrInvalidInput
	}
	return SubscriptionInput{Endpoint: endpoint, P256DH: p256dh, Auth: auth, UserAgent: userAgent}, nil
}

func validSubscriptionKey(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > maxKeyRunes {
		return false
	}
	if _, err := base64.RawURLEncoding.DecodeString(value); err == nil {
		return true
	}
	_, err := base64.URLEncoding.DecodeString(value)
	return err == nil
}

func subscriptionFromModel(row db.PushSubscription) Subscription {
	return Subscription{
		ID:        uuidString(row.ID),
		Endpoint:  row.Endpoint,
		UserAgent: textPtr(row.UserAgent),
		CreatedAt: timestamp(row.CreatedAt),
		UpdatedAt: timestamp(row.UpdatedAt),
	}
}

func parseActor(actor Actor) (pgtype.UUID, error) {
	id, err := parseUUID(actor.ID)
	if err != nil {
		return pgtype.UUID{}, ErrUnauthorized
	}
	return id, nil
}

func mapWriteError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case database.IsUniqueViolation(err), database.IsForeignKeyViolation(err),
		database.IsCheckViolation(err), database.IsNotNullViolation(err):
		return ErrInvalidInput
	default:
		return err
	}
}
