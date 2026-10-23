package reporting

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

func (s *Service) ListAuditLogs(ctx context.Context, actor Actor, filter AuditLogFilter) (AuditLogResult, error) {
	if !hasRole(actor.Roles, "admin") {
		return AuditLogResult{}, ErrForbidden
	}
	action := strings.TrimSpace(filter.Action)
	entityType := strings.TrimSpace(filter.EntityType)
	if len(action) > 100 || len(entityType) > 50 {
		return AuditLogResult{}, ErrInvalidInput
	}
	entityID, err := optionalUUID(filter.EntityID)
	if err != nil {
		return AuditLogResult{}, ErrInvalidInput
	}
	actorID, err := optionalUUID(filter.ActorID)
	if err != nil {
		return AuditLogResult{}, ErrInvalidInput
	}
	from, err := optionalDate(filter.From)
	if err != nil {
		return AuditLogResult{}, err
	}
	to, err := optionalDate(filter.To)
	if err != nil {
		return AuditLogResult{}, err
	}
	if from.Valid && to.Valid && to.Time.Before(from.Time) {
		return AuditLogResult{}, ErrInvalidInput
	}
	var fromAt, toAt pgtype.Timestamptz
	if from.Valid || to.Valid {
		loc, err := s.schoolLocation(ctx)
		if err != nil {
			return AuditLogResult{}, fmt.Errorf("resolve school timezone for audit logs: %w", err)
		}
		if from.Valid {
			fromAt = timestampAt(dayStart(from, loc))
		}
		if to.Valid {
			toAt = timestampAt(dayStart(to, loc).AddDate(0, 0, 1))
		}
	}
	limit, offset := normalizePage(filter.Limit, filter.Offset)
	params := db.ListAuditLogsParams{
		Action:     optionalText(action),
		EntityType: optionalText(entityType),
		EntityID:   entityID,
		ActorID:    actorID,
		FromAt:     fromAt,
		ToAt:       toAt,
		PageOffset: offset,
		PageLimit:  limit,
	}
	rows, err := s.queries.ListAuditLogs(ctx, params)
	if err != nil {
		return AuditLogResult{}, fmt.Errorf("list audit logs: %w", err)
	}
	total, err := s.queries.CountAuditLogs(ctx, db.CountAuditLogsParams{
		Action:     params.Action,
		EntityType: params.EntityType,
		EntityID:   params.EntityID,
		ActorID:    params.ActorID,
		FromAt:     params.FromAt,
		ToAt:       params.ToAt,
	})
	if err != nil {
		return AuditLogResult{}, fmt.Errorf("count audit logs: %w", err)
	}
	items := make([]AuditLog, 0, len(rows))
	for _, row := range rows {
		items = append(items, AuditLog{
			ID:          uuidString(row.ID),
			ActorUserID: uuidString(row.ActorUserID),
			ActorName:   row.ActorName,
			Action:      row.Action,
			EntityType:  row.EntityType,
			EntityID:    uuidString(row.EntityID),
			RequestID:   row.RequestID,
			BeforeData:  extractJSON(row.BeforeData),
			AfterData:   extractJSON(row.AfterData),
			Reason:      row.Reason,
			CreatedAt:   timestamp(row.CreatedAt),
		})
	}
	return AuditLogResult{Data: items, Total: total, Limit: limit, Offset: offset}, nil
}

func optionalDate(value string) (pgtype.Date, error) {
	if strings.TrimSpace(value) == "" {
		return pgtype.Date{}, nil
	}
	date, err := parseDate(value)
	if err != nil {
		return pgtype.Date{}, ErrInvalidInput
	}
	return date, nil
}
