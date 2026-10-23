package attendance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const autoCloseInterval = time.Minute

func (s *Service) RunAutoClose(ctx context.Context) {
	ticker := time.NewTicker(autoCloseInterval)
	defer ticker.Stop()

	if err := s.AutoCloseDue(ctx, time.Now()); err != nil {
		s.log.Error("attendance_auto_close_failed", "error", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.AutoCloseDue(ctx, time.Now()); err != nil {
				s.log.Error("attendance_auto_close_failed", "error", err)
			}
		}
	}
}

func (s *Service) AutoCloseDue(ctx context.Context, now time.Time) error {
	settings, err := s.queries.GetSchoolSettings(ctx)
	if err != nil {
		return fmt.Errorf("get school settings: %w", err)
	}
	loc, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return fmt.Errorf("load school timezone: %w", err)
	}
	local := now.In(loc)
	closeAt, err := time.Parse("15:04", timeOfDayString(settings.AttendanceAutoCloseTime))
	if err != nil {
		return fmt.Errorf("parse attendance auto close time: %w", err)
	}
	if local.Hour() < closeAt.Hour() || (local.Hour() == closeAt.Hour() && local.Minute() < closeAt.Minute()) {
		return nil
	}
	date := pgtype.Date{
		Time:  time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
	candidates, err := s.queries.ListClassesDueForAutoClose(ctx, date)
	if err != nil {
		return fmt.Errorf("list classes due for auto close: %w", err)
	}
	if len(candidates) == 0 {
		return nil
	}
	systemID, err := s.queries.GetSystemUserID(ctx)
	if err != nil {
		return fmt.Errorf("get system user: %w", err)
	}
	for _, classID := range candidates {
		_, _, err := s.closeDay(ctx, classID, date, loc, "auto", systemID, nil)
		if err != nil {
			if errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalidInput) || errors.Is(err, ErrNotFound) {
				continue
			}
			return fmt.Errorf("auto close class %s: %w", uuidString(classID), err)
		}
	}
	return nil
}
