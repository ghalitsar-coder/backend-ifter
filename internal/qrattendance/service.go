package qrattendance

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

const dateLayout = "2006-01-02"

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

func (s *Service) ActiveSession(ctx context.Context, schoolID int64) (*Session, error) {
	if schoolID <= 0 {
		return nil, ErrForbidden
	}
	now := s.now()
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	session, err := s.repo.FindOpenSession(ctx, schoolID, date, now.Format("15:04:05"))
	if database.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find active QR session: %w", err)
	}
	return &session.Session, nil
}

func (s *Service) CreateSession(ctx context.Context, creator SessionCreator, input CreateSessionInput) (Session, error) {
	if creator.UserID <= 0 || creator.SchoolID <= 0 {
		return Session{}, ErrForbidden
	}
	date, err := time.Parse(dateLayout, input.Date)
	if err != nil {
		return Session{}, ErrInvalidDate
	}
	if input.Type != "masuk" && input.Type != "pulang" {
		return Session{}, ErrInvalidSession
	}
	openAt, err := normalizeClock(input.OpenAt)
	if err != nil {
		return Session{}, ErrInvalidTime
	}
	closeAt, err := normalizeClock(input.CloseAt)
	if err != nil || closeAt <= openAt {
		return Session{}, ErrInvalidTime
	}
	var lateAfter *string
	if input.LateAfter != nil {
		if input.Type != "masuk" {
			return Session{}, ErrInvalidTime
		}
		value, err := normalizeClock(*input.LateAfter)
		if err != nil || value < openAt || value > closeAt {
			return Session{}, ErrInvalidTime
		}
		lateAfter = &value
	}
	rotation := 30
	if input.RotationSeconds != nil {
		rotation = *input.RotationSeconds
	}
	if rotation < 0 || rotation > 3600 {
		return Session{}, ErrInvalidRotation
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return Session{}, fmt.Errorf("generate QR secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	created, err := s.repo.CreateSession(ctx, creator.SchoolID, creator.UserID, secret, input, date, openAt, closeAt, lateAfter)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return Session{}, ErrSessionExists
		}
		return Session{}, fmt.Errorf("create QR session: %w", err)
	}
	return created.Session, nil
}

func (s *Service) CurrentCode(ctx context.Context, id, schoolID int64) (QRCode, error) {
	session, err := s.getSession(ctx, id, schoolID)
	if err != nil {
		return QRCode{}, err
	}
	now := s.now()
	if err := validateWindow(session, now); err != nil {
		return QRCode{}, err
	}
	rotation := int64(session.RotationSeconds)
	slot := int64(0)
	validUntil := sessionEnd(session, now)
	if rotation > 0 {
		slot = now.Unix() / rotation
		validUntil = time.Unix((slot+1)*rotation, 0).In(now.Location())
		if end := sessionEnd(session, now); end.Before(validUntil) {
			validUntil = end
		}
	}
	payload := signPayload(session.ID, slot, session.TokenSecret)
	png, err := qrcode.Encode(payload, qrcode.Medium, 512)
	if err != nil {
		return QRCode{}, fmt.Errorf("encode attendance QR: %w", err)
	}
	return QRCode{
		SessionID:  id,
		Payload:    payload,
		ImagePNG:   "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		ValidUntil: validUntil,
	}, nil
}

func (s *Service) Scan(ctx context.Context, input ScanInput) (ScanResult, error) {
	if input.UserID <= 0 || input.SchoolID <= 0 {
		return ScanResult{}, ErrForbidden
	}
	sessionID, slot, signature, err := parsePayload(input.Payload)
	if err != nil {
		return ScanResult{}, ErrInvalidCode
	}
	session, err := s.getSession(ctx, sessionID, input.SchoolID)
	if err != nil {
		return ScanResult{}, err
	}
	if !validSignature(session, slot, signature) {
		return ScanResult{}, ErrInvalidCode
	}
	now := s.now()
	if err := validateWindow(session, now); err != nil {
		return ScanResult{}, err
	}
	if !validSlot(session, slot, now) {
		if session.RotationSeconds > 0 && slot < now.Unix()/int64(session.RotationSeconds) {
			return ScanResult{}, ErrExpiredCode
		}
		return ScanResult{}, ErrInvalidCode
	}
	staffID, err := s.repo.FindActiveStaff(ctx, input.UserID, input.SchoolID)
	if database.IsNotFound(err) {
		return ScanResult{}, ErrForbidden
	}
	if err != nil {
		return ScanResult{}, fmt.Errorf("resolve scanning staff: %w", err)
	}

	date, _ := time.ParseInLocation(dateLayout, session.Date, now.Location())
	result := ScanResult{StaffID: staffID, Type: session.Type, Date: session.Date, ScannedAt: now}
	if session.Type == "masuk" {
		result.Status = "hadir"
		if session.LateAfter != nil && now.Format("15:04:05") >= *session.LateAfter {
			result.Status = "terlambat"
		}
		err = s.repo.CheckIn(ctx, staffID, session.ID, date, now, result.Status)
		if database.IsNotFound(err) {
			return ScanResult{}, ErrAlreadyScanned
		}
	} else {
		result.Status = "pulang"
		err = s.repo.CheckOut(ctx, staffID, session.ID, date, now)
		if database.IsNotFound(err) {
			return ScanResult{}, ErrCheckInRequired
		}
	}
	if err != nil {
		return ScanResult{}, fmt.Errorf("record staff QR attendance: %w", err)
	}
	return result, nil
}

func (s *Service) CloseSession(ctx context.Context, id, schoolID int64) error {
	if id <= 0 || schoolID <= 0 {
		return ErrInvalidSession
	}
	if err := s.repo.CloseSession(ctx, id, schoolID); err != nil {
		if database.IsNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("close QR session: %w", err)
	}
	return nil
}

func (s *Service) getSession(ctx context.Context, id, schoolID int64) (SessionRecord, error) {
	if id <= 0 || schoolID <= 0 {
		return SessionRecord{}, ErrInvalidSession
	}
	session, err := s.repo.GetSession(ctx, id, schoolID)
	if database.IsNotFound(err) {
		return SessionRecord{}, ErrNotFound
	}
	if err != nil {
		return SessionRecord{}, fmt.Errorf("get QR session: %w", err)
	}
	return session, nil
}

func validateWindow(session SessionRecord, now time.Time) error {
	if session.Status != "aktif" {
		return ErrSessionClosed
	}
	if now.Format(dateLayout) != session.Date {
		return ErrOutsideWindow
	}
	clock := now.Format("15:04:05")
	if clock < session.OpenAt || clock > session.CloseAt {
		return ErrOutsideWindow
	}
	return nil
}

func sessionEnd(session SessionRecord, now time.Time) time.Time {
	end, _ := time.ParseInLocation("15:04:05", session.CloseAt, now.Location())
	return time.Date(now.Year(), now.Month(), now.Day(), end.Hour(), end.Minute(), end.Second(), 0, now.Location())
}

func normalizeClock(value string) (string, error) {
	for _, layout := range []string{"15:04", "15:04:05"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format("15:04:05"), nil
		}
	}
	return "", ErrInvalidTime
}

func signPayload(sessionID, slot int64, secret string) string {
	message := fmt.Sprintf("v1:%d:%d", sessionID, slot)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	return fmt.Sprintf("v1.%d.%d.%s", sessionID, slot, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}

func parsePayload(payload string) (int64, int64, []byte, error) {
	parts := strings.Split(payload, ".")
	if len(parts) != 4 || parts[0] != "v1" {
		return 0, 0, nil, ErrInvalidCode
	}
	sessionID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || sessionID <= 0 {
		return 0, 0, nil, ErrInvalidCode
	}
	slot, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || slot < 0 {
		return 0, 0, nil, ErrInvalidCode
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(signature) != sha256.Size {
		return 0, 0, nil, ErrInvalidCode
	}
	return sessionID, slot, signature, nil
}

func validSignature(session SessionRecord, slot int64, signature []byte) bool {
	parts := strings.Split(signPayload(session.ID, slot, session.TokenSecret), ".")
	expected, _ := base64.RawURLEncoding.DecodeString(parts[3])
	return hmac.Equal(signature, expected)
}

func validSlot(session SessionRecord, slot int64, now time.Time) bool {
	rotation := int64(session.RotationSeconds)
	if rotation == 0 {
		return slot == 0
	}
	current := now.Unix() / rotation
	// Allow a photo taken just before rotation to reach the server.
	return slot == current || (slot == current-1 && now.Unix()%rotation < 15)
}
