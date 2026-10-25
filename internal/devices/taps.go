package devices

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

func normalizeCardUID(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" ||
		utf8.RuneCountInString(value) > maxCardUIDRunes ||
		strings.IndexByte(value, 0) >= 0 ||
		strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", ErrInvalidInput
	}
	return value, nil
}

func parseDateFilter(value string) (pgtype.Date, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Date{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return pgtype.Date{}, ErrInvalidInput
	}
	return pgtype.Date{Time: parsed, Valid: true}, nil
}

// IngestTaps accepts a reader batch (POST /device-taps). The device is
// resolved from the bearer token, not from the payload; the payload's
// identifier must still match so a copied token fails. Taps are idempotent per
// (device, card, occurred_at), and taps for unmapped cards are kept with a
// NULL student so a later card mapping can explain them.
func (s *Service) IngestTaps(ctx context.Context, token string, batch TapBatch) (TapIngestResult, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return TapIngestResult{}, ErrInvalidToken
	}
	device, err := s.queries.GetDeviceByIngestTokenHash(ctx, optionalText(hashIngestToken(token)))
	if err != nil {
		if database.IsNotFound(err) {
			return TapIngestResult{}, ErrInvalidToken
		}
		return TapIngestResult{}, fmt.Errorf("get device by token: %w", err)
	}
	if device.Status != StatusActive {
		return TapIngestResult{}, ErrDeviceInactive
	}
	identifier := strings.TrimSpace(batch.Identifier)
	if identifier == "" || !strings.EqualFold(identifier, device.Identifier) {
		return TapIngestResult{}, ErrInvalidToken
	}
	if len(batch.Taps) < 1 || len(batch.Taps) > maxTapBatchSize {
		return TapIngestResult{}, ErrInvalidInput
	}
	limit := time.Now().Add(tapFutureSkew)
	type normalizedTap struct {
		cardUID    string
		occurredAt time.Time
	}
	taps := make([]normalizedTap, 0, len(batch.Taps))
	for _, tap := range batch.Taps {
		cardUID, err := normalizeCardUID(tap.CardUID)
		if err != nil {
			return TapIngestResult{}, ErrInvalidInput
		}
		if tap.OccurredAt.IsZero() || tap.OccurredAt.After(limit) {
			return TapIngestResult{}, ErrInvalidInput
		}
		taps = append(taps, normalizedTap{cardUID: cardUID, occurredAt: tap.OccurredAt})
	}

	result := TapIngestResult{}
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		for _, tap := range taps {
			var studentID pgtype.UUID
			student, err := q.GetStudentByCardUID(ctx, tap.cardUID)
			if err != nil {
				if !database.IsNotFound(err) {
					return fmt.Errorf("resolve student card: %w", err)
				}
			} else {
				studentID = student.ID
			}
			inserted, err := q.CreateDeviceTap(ctx, db.CreateDeviceTapParams{
				DeviceID:   device.ID,
				StudentID:  studentID,
				CardUid:    tap.cardUID,
				OccurredAt: pgtype.Timestamptz{Time: tap.occurredAt, Valid: true},
			})
			if err != nil {
				return mapWriteError(fmt.Errorf("create device tap: %w", err))
			}
			if inserted == 0 {
				result.Duplicates++
				continue
			}
			result.Accepted++
			if !studentID.Valid {
				result.Unknown++
			}
		}
		if err := q.TouchDeviceLastSeen(ctx, device.ID); err != nil {
			return fmt.Errorf("touch device last seen: %w", err)
		}
		return nil
	})
	if err != nil {
		return TapIngestResult{}, err
	}
	return result, nil
}

// ListTaps returns the tap log for staff oversight (GET /device-taps).
func (s *Service) ListTaps(ctx context.Context, actor Actor, filter TapFilter) (TapList, error) {
	if !hasRole(actor.Roles, "admin") && !hasRole(actor.Roles, "duty_teacher") && !hasRole(actor.Roles, "principal") {
		return TapList{}, ErrForbidden
	}
	params := db.ListDeviceTapsParams{
		PageLimit:  filter.Limit,
		PageOffset: filter.Offset,
	}
	if params.PageLimit <= 0 {
		params.PageLimit = defaultTapLimit
	}
	if params.PageLimit > maxTapLimit || params.PageOffset < 0 {
		return TapList{}, ErrInvalidInput
	}
	if strings.TrimSpace(filter.DeviceID) != "" {
		id, err := parseUUID(filter.DeviceID)
		if err != nil {
			return TapList{}, ErrInvalidInput
		}
		params.DeviceID = id
	}
	if strings.TrimSpace(filter.StudentID) != "" {
		id, err := parseUUID(filter.StudentID)
		if err != nil {
			return TapList{}, ErrInvalidInput
		}
		params.StudentID = id
	}
	if strings.TrimSpace(filter.CardUID) != "" {
		cardUID, err := normalizeCardUID(filter.CardUID)
		if err != nil {
			return TapList{}, ErrInvalidInput
		}
		params.CardUid = optionalText(cardUID)
	}
	from, err := parseDateFilter(filter.From)
	if err != nil {
		return TapList{}, err
	}
	to, err := parseDateFilter(filter.To)
	if err != nil {
		return TapList{}, err
	}
	if from.Valid && to.Valid && from.Time.After(to.Time) {
		return TapList{}, ErrInvalidInput
	}
	params.FromDate = from
	params.ToDate = to

	total, err := s.queries.CountDeviceTaps(ctx, db.CountDeviceTapsParams{
		DeviceID:  params.DeviceID,
		StudentID: params.StudentID,
		CardUid:   params.CardUid,
		FromDate:  params.FromDate,
		ToDate:    params.ToDate,
	})
	if err != nil {
		return TapList{}, fmt.Errorf("count device taps: %w", err)
	}
	rows, err := s.queries.ListDeviceTaps(ctx, params)
	if err != nil {
		return TapList{}, fmt.Errorf("list device taps: %w", err)
	}
	items := make([]Tap, 0, len(rows))
	for _, row := range rows {
		items = append(items, tapFromRow(row))
	}
	return TapList{Data: items, Total: total, Limit: params.PageLimit, Offset: params.PageOffset}, nil
}

func tapFromRow(row db.ListDeviceTapsRow) Tap {
	tap := Tap{
		ID:               uuidString(row.ID),
		DeviceID:         uuidString(row.DeviceID),
		DeviceName:       row.DeviceName,
		DeviceIdentifier: row.DeviceIdentifier,
		CardUID:          row.CardUid,
		OccurredAt:       timestamp(row.OccurredAt),
		CreatedAt:        timestamp(row.CreatedAt),
	}
	if row.StudentID.Valid {
		id := uuidString(row.StudentID)
		tap.StudentID = &id
	}
	tap.StudentNumber = textPtr(row.StudentNumber)
	tap.StudentName = textPtr(row.FullName)
	return tap
}
