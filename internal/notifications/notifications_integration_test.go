package notifications_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	notificationspkg "github.com/ghalitsar-coder/backend-ifter/internal/notifications"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type testData struct {
	adminID        string
	adminToken     string
	principalID    string
	principalToken string
	teacherToken   string
	studentID      string
	studentToken   string
}

func TestPushAPIDisabled(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	rec := env.request(t, http.MethodGet, "/api/v1/push/config", nil, data.adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var view notificationspkg.ConfigView
	decode(t, rec, &view)
	if view.Enabled || view.PublicKey != "" {
		t.Fatalf("disabled config = %+v", view)
	}

	if rec := env.request(t, http.MethodPost, "/api/v1/push/subscriptions", validSubscription(), data.adminToken); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("subscribe status = %d, body = %s", rec.Code, rec.Body.String())
	} else {
		var body struct {
			Code string `json:"code"`
		}
		decode(t, rec, &body)
		if body.Code != "push_disabled" {
			t.Fatalf("subscribe error code = %q", body.Code)
		}
	}
	if rec := env.request(t, http.MethodPost, "/api/v1/admin/push/test", map[string]any{
		"title": "Uji", "body": "Halo",
	}, data.adminToken); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("test-send status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = env.request(t, http.MethodGet, "/api/v1/push/subscriptions", nil, data.studentToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Data []notificationspkg.Subscription `json:"data"`
	}
	decode(t, rec, &list)
	if len(list.Data) != 0 {
		t.Fatalf("empty list = %+v", list.Data)
	}
	if rec := env.request(t, http.MethodDelete, "/api/v1/push/subscriptions/00000000-0000-0000-0000-000000000000", nil, data.adminToken); rec.Code != http.StatusNotFound {
		t.Fatalf("delete unknown status = %d", rec.Code)
	}
	if rec := env.request(t, http.MethodGet, "/api/v1/push/config", nil, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous config status = %d", rec.Code)
	}
}

func TestPushAPIEnabled(t *testing.T) {
	setEnabledEnv(t)
	env := newAPIEnv(t)
	data := env.seed(t)

	rec := env.request(t, http.MethodGet, "/api/v1/push/config", nil, data.studentToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var view notificationspkg.ConfigView
	decode(t, rec, &view)
	if !view.Enabled || view.PublicKey == "" {
		t.Fatalf("enabled config = %+v", view)
	}

	var adminSubscriptionID, studentSubscriptionID string
	t.Run("validation", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"blank endpoint":    {"endpoint": " ", "p256dh": key65(), "auth": key16()},
			"http endpoint":     {"endpoint": "http://push.example.test/x", "p256dh": key65(), "auth": key16()},
			"loopback endpoint": {"endpoint": "https://127.0.0.1/x", "p256dh": key65(), "auth": key16()},
			"invalid key":       {"endpoint": "https://push.example.test/x", "p256dh": "not base64!!", "auth": key16()},
			"blank auth":        {"endpoint": "https://push.example.test/x", "p256dh": key65(), "auth": ""},
		} {
			if rec := env.request(t, http.MethodPost, "/api/v1/push/subscriptions", body, data.studentToken); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("subscribe list and upsert", func(t *testing.T) {
		body := validSubscription()
		body["user_agent"] = "Firefox/128"
		rec := env.request(t, http.MethodPost, "/api/v1/push/subscriptions", body, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("subscribe status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var created struct {
			Subscription map[string]any `json:"subscription"`
		}
		decode(t, rec, &created)
		adminSubscriptionID, _ = created.Subscription["id"].(string)
		if adminSubscriptionID == "" || created.Subscription["user_agent"] != "Firefox/128" {
			t.Fatalf("created subscription = %+v", created.Subscription)
		}
		if _, leaked := created.Subscription["p256dh"]; leaked {
			t.Fatalf("response leaks p256dh: %+v", created.Subscription)
		}
		if _, leaked := created.Subscription["auth"]; leaked {
			t.Fatalf("response leaks auth: %+v", created.Subscription)
		}

		// Same endpoint upserts for the same browser with the new user agent.
		body["user_agent"] = "Firefox/129"
		rec = env.request(t, http.MethodPost, "/api/v1/push/subscriptions", body, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upsert status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var upserted struct {
			Subscription map[string]any `json:"subscription"`
		}
		decode(t, rec, &upserted)
		if upserted.Subscription["id"] != adminSubscriptionID || upserted.Subscription["user_agent"] != "Firefox/129" {
			t.Fatalf("upserted subscription = %+v", upserted.Subscription)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/push/subscriptions", nil, data.adminToken)
		var list struct {
			Data []notificationspkg.Subscription `json:"data"`
		}
		decode(t, rec, &list)
		if len(list.Data) != 1 || list.Data[0].ID != adminSubscriptionID {
			t.Fatalf("admin list = %+v", list.Data)
		}
	})

	t.Run("endpoint ownership moves on re-subscribe", func(t *testing.T) {
		body := validSubscription()
		rec := env.request(t, http.MethodPost, "/api/v1/push/subscriptions", body, data.studentToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("student subscribe status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var created struct {
			Subscription notificationspkg.Subscription `json:"subscription"`
		}
		decode(t, rec, &created)
		studentSubscriptionID = created.Subscription.ID
		if studentSubscriptionID != adminSubscriptionID {
			t.Fatalf("upsert should keep the row id: student=%s admin=%s", studentSubscriptionID, adminSubscriptionID)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/push/subscriptions", nil, data.adminToken)
		var list struct {
			Data []notificationspkg.Subscription `json:"data"`
		}
		decode(t, rec, &list)
		if len(list.Data) != 0 {
			t.Fatalf("admin still owns moved subscription: %+v", list.Data)
		}
	})

	t.Run("delete is own-only", func(t *testing.T) {
		if rec := env.request(t, http.MethodDelete, "/api/v1/push/subscriptions/"+studentSubscriptionID, nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("cross-user delete status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodDelete, "/api/v1/push/subscriptions/"+studentSubscriptionID, nil, data.studentToken); rec.Code != http.StatusNoContent {
			t.Fatalf("own delete status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("test send rbac and empty result", func(t *testing.T) {
		for name, token := range map[string]string{
			"principal": data.principalToken,
			"teacher":   data.teacherToken,
			"student":   data.studentToken,
		} {
			if rec := env.request(t, http.MethodPost, "/api/v1/admin/push/test", map[string]any{
				"title": "Uji", "body": "Halo",
			}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s test-send status = %d", name, rec.Code)
			}
		}
		// The admin has no subscription left, so nothing is delivered.
		rec := env.request(t, http.MethodPost, "/api/v1/admin/push/test", map[string]any{
			"title": "Uji", "body": "Halo",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin test-send status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var result notificationspkg.SendResult
		decode(t, rec, &result)
		if result.Sent != 0 || result.Failed != 0 {
			t.Fatalf("test-send result = %+v", result)
		}
	})

	t.Run("service send paths use the injected sender", func(t *testing.T) {
		fake := &fakeSender{statusByEndpoint: map[string]int{}}
		service := notificationspkg.NewServiceWithSender(env.pool, discardLogger(), enabledConfig(), fake)
		ctx := context.Background()

		admin := notificationspkg.Actor{ID: data.adminID, Roles: []string{"admin"}}
		principal := notificationspkg.Actor{ID: data.principalID, Roles: []string{"principal"}}
		student := notificationspkg.Actor{ID: data.studentID, Roles: []string{"student"}}
		for _, seed := range []struct {
			actor    notificationspkg.Actor
			endpoint string
		}{
			{admin, "https://push.example.test/admin"},
			{principal, "https://push.example.test/principal"},
			{student, "https://push.example.test/student"},
		} {
			if _, err := service.Subscribe(ctx, seed.actor, notificationspkg.SubscriptionInput{
				Endpoint: seed.endpoint, P256DH: key65(), Auth: key16(),
			}); err != nil {
				t.Fatalf("seed subscription: %v", err)
			}
		}

		result, err := service.NotifyRoles(ctx, []string{"admin", "principal"}, notificationspkg.Payload{
			Title: "Pelanggaran baru", Body: "Ani — Terlambat (5 poin)", URL: "/dashboard/kedisiplinan",
		})
		if err != nil {
			t.Fatalf("notify roles: %v", err)
		}
		if result.Sent != 2 || result.Failed != 0 || len(fake.calls) != 2 {
			t.Fatalf("notify result = %+v, calls = %d", result, len(fake.calls))
		}
		for _, call := range fake.calls {
			if call.keys.Endpoint == "https://push.example.test/student" {
				t.Fatalf("student subscription received a staff notification")
			}
			var payload notificationspkg.Payload
			if err := json.Unmarshal(call.payload, &payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if payload.Title != "Pelanggaran baru" || payload.URL != "/dashboard/kedisiplinan" ||
				payload.Body != "Ani — Terlambat (5 poin)" {
				t.Fatalf("payload = %+v", payload)
			}
		}

		// A 410 Gone subscription is pruned after the failed send.
		if _, err := service.Subscribe(ctx, admin, notificationspkg.SubscriptionInput{
			Endpoint: "https://push.example.test/gone", P256DH: key65(), Auth: key16(),
		}); err != nil {
			t.Fatalf("seed gone subscription: %v", err)
		}
		fake.statusByEndpoint["https://push.example.test/gone"] = http.StatusGone
		result, err = service.NotifyRoles(ctx, []string{"admin"}, notificationspkg.Payload{
			Title: "Uji", Body: "Pembersihan",
		})
		if err != nil {
			t.Fatalf("notify roles after gone: %v", err)
		}
		if result.Sent != 1 || result.Failed != 1 {
			t.Fatalf("gone result = %+v", result)
		}
		subscriptions, err := service.List(ctx, admin)
		if err != nil {
			t.Fatalf("list after prune: %v", err)
		}
		for _, subscription := range subscriptions {
			if subscription.Endpoint == "https://push.example.test/gone" {
				t.Fatalf("410 subscription was not pruned")
			}
		}

		if _, err := service.SendTest(ctx, student, notificationspkg.SendInput{Title: "x", Body: "y"}); err != notificationspkg.ErrForbidden {
			t.Fatalf("student SendTest error = %v", err)
		}
		if _, err := service.Subscribe(ctx, notificationspkg.Actor{ID: data.adminID}, notificationspkg.SubscriptionInput{
			Endpoint: "http://push.example.test/x", P256DH: key65(), Auth: key16(),
		}); err != notificationspkg.ErrInvalidInput {
			t.Fatalf("http endpoint error = %v", err)
		}
	})

	t.Run("disabled service rejects writes but serves config", func(t *testing.T) {
		service := notificationspkg.NewServiceWithSender(env.pool, discardLogger(), notificationspkg.Config{}, &fakeSender{})
		if cfg := service.Config(); cfg.Enabled || cfg.PublicKey != "" {
			t.Fatalf("disabled config = %+v", cfg)
		}
		actor := notificationspkg.Actor{ID: data.adminID, Roles: []string{"admin"}}
		if _, err := service.Subscribe(ctxBackground(), actor, notificationspkg.SubscriptionInput{
			Endpoint: "https://push.example.test/x", P256DH: key65(), Auth: key16(),
		}); err != notificationspkg.ErrDisabled {
			t.Fatalf("disabled subscribe error = %v", err)
		}
		if _, err := service.SendTest(ctxBackground(), actor, notificationspkg.SendInput{Title: "x", Body: "y"}); err != notificationspkg.ErrDisabled {
			t.Fatalf("disabled SendTest error = %v", err)
		}

		// An enabled flag with an incomplete VAPID triple stays inert.
		incomplete := notificationspkg.NewServiceWithSender(env.pool, discardLogger(), notificationspkg.Config{Enabled: true}, &fakeSender{})
		if cfg := incomplete.Config(); cfg.Enabled {
			t.Fatalf("incomplete config should stay disabled")
		}
	})
}

// fakeSender records pushes; endpoints listed in statusByEndpoint fail with
// that status and a synthetic error.
type fakeSender struct {
	calls            []capturedPush
	statusByEndpoint map[string]int
}

type capturedPush struct {
	keys    notificationspkg.SubscriptionKeys
	payload []byte
}

func (f *fakeSender) Send(_ context.Context, keys notificationspkg.SubscriptionKeys, payload []byte) (int, error) {
	f.calls = append(f.calls, capturedPush{keys: keys, payload: payload})
	if status, ok := f.statusByEndpoint[keys.Endpoint]; ok {
		return status, fmt.Errorf("push service answered %d", status)
	}
	return http.StatusCreated, nil
}

func enabledConfig() notificationspkg.Config {
	return notificationspkg.Config{
		Enabled:    true,
		PublicKey:  key65(),
		PrivateKey: key32(),
		Subject:    "mailto:test@example.com",
	}
}

func setEnabledEnv(t *testing.T) {
	t.Helper()
	t.Setenv("IFTER_WEBPUSH_ENABLED", "true")
	t.Setenv("IFTER_WEBPUSH_PUBLIC_KEY", key65())
	t.Setenv("IFTER_WEBPUSH_PRIVATE_KEY", key32())
	t.Setenv("IFTER_WEBPUSH_SUBJECT", "mailto:test@example.com")
}

func validSubscription() map[string]any {
	return map[string]any{
		"endpoint": "https://push.example.test/subscription/abc",
		"p256dh":   key65(),
		"auth":     key16(),
	}
}

func key65() string { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 65)) }
func key32() string { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)) }
func key16() string { return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{3}, 16)) }

func ctxBackground() context.Context { return context.Background() }

func newAPIEnv(t *testing.T) apiEnv {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	adminConfig, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	name := fmt.Sprintf("ifter_push_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		admin.Close()
	})
	testConfig, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}
	testConfig.ConnConfig.Database = name
	testConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err = pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	paths, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
	log := discardLogger()
	tokens := authpkg.NewTokenManager("test-secret-at-least-32-characters-long", "test-issuer", 15*time.Minute)
	authService := authpkg.NewService(pool, tokens, authpkg.ServiceConfig{
		RefreshTokenTTL: 24 * time.Hour,
		InviteTokenTTL:  72 * time.Hour,
		ResetTokenTTL:   time.Hour,
		BcryptCost:      bcrypt.MinCost,
	}, log)
	router := httptransport.New(log, pool, authService, nil, nil, nil, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	adminID := e.insertUser(t, "push-admin", "push-admin@example.com", "push-admin-pw", "admin")
	principalID := e.insertUser(t, "push-principal", "push-principal@example.com", "push-principal-pw", "principal")
	e.insertUser(t, "push-teacher", "push-teacher@example.com", "push-teacher-pw", "teacher")
	studentID := e.insertUser(t, "push-student", "push-student@example.com", "push-student-pw", "student")

	return testData{
		adminID:        adminID,
		adminToken:     e.login(t, "push-admin", "push-admin-pw"),
		principalID:    principalID,
		principalToken: e.login(t, "push-principal", "push-principal-pw"),
		teacherToken:   e.login(t, "push-teacher", "push-teacher-pw"),
		studentID:      studentID,
		studentToken:   e.login(t, "push-student", "push-student-pw"),
	}
}

func (e apiEnv) insertUser(t *testing.T, username, email, password string, roles ...string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash, full_name, status)
		 VALUES ($1, $2, $3, $4, 'active') RETURNING id::text`,
		username, email, string(hash), "Test "+username,
	).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
	for _, role := range roles {
		if _, err := e.pool.Exec(context.Background(),
			"INSERT INTO user_roles (user_id, role_code) VALUES ($1,$2)", id, role,
		); err != nil {
			t.Fatalf("assign role %q: %v", role, err)
		}
	}
	return id
}

func (e apiEnv) login(t *testing.T, username, password string) string {
	t.Helper()
	rec := e.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": username, "password": password,
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s status = %d, body = %s", username, rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	decode(t, rec, &body)
	return body.AccessToken
}

func (e apiEnv) request(t *testing.T, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
