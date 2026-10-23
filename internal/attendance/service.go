package attendance

import (
	"context"
	"fmt"
	"log/slog"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	log     *slog.Logger
}

func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{
		pool:    pool,
		queries: db.New(pool),
		log:     log,
	}
}

func (s *Service) GetSettings(ctx context.Context) (SettingsDetail, error) {
	settings, err := s.queries.GetSchoolSettings(ctx)
	if err != nil {
		if database.IsNotFound(err) {
			return SettingsDetail{}, ErrNotFound
		}
		return SettingsDetail{}, fmt.Errorf("get school settings: %w", err)
	}
	schedules, err := s.queries.ListSchoolSchedules(ctx)
	if err != nil {
		return SettingsDetail{}, fmt.Errorf("list school schedules: %w", err)
	}
	holidays, err := s.queries.ListSchoolHolidays(ctx, db.ListSchoolHolidaysParams{})
	if err != nil {
		return SettingsDetail{}, fmt.Errorf("list school holidays: %w", err)
	}
	return settingsDetail(settings, schedules, holidays), nil
}

func (s *Service) UpdateSettings(ctx context.Context, actor Actor, input SettingsInput, requestID string) (SettingsDetail, error) {
	if !hasRole(actor.Roles, "admin") {
		return SettingsDetail{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return SettingsDetail{}, ErrUnauthorized
	}
	schoolName, err := requiredText(input.SchoolName, 150)
	if err != nil {
		return SettingsDetail{}, err
	}
	if len(input.Timezone) > 50 {
		return SettingsDetail{}, ErrInvalidInput
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return SettingsDetail{}, ErrInvalidInput
	}
	autoClose, err := parseTimeOfDay(input.AutoCloseTime)
	if err != nil {
		return SettingsDetail{}, err
	}
	if len(input.Schedules) > 7 {
		return SettingsDetail{}, ErrInvalidInput
	}
	seen := make(map[int16]struct{}, len(input.Schedules))
	schedules := make([]ScheduleInput, 0, len(input.Schedules))
	for _, item := range input.Schedules {
		if item.Weekday < 1 || item.Weekday > 7 || item.LateToleranceMinutes < 0 || item.LateToleranceMinutes > 240 {
			return SettingsDetail{}, ErrInvalidInput
		}
		if _, exists := seen[item.Weekday]; exists {
			return SettingsDetail{}, ErrInvalidInput
		}
		seen[item.Weekday] = struct{}{}
		if _, err := parseTimeOfDay(item.StartTime); err != nil {
			return SettingsDetail{}, err
		}
		schedules = append(schedules, item)
	}

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		beforeSettings, err := q.GetSchoolSettings(ctx)
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get settings before update: %w", err)
		}
		beforeSchedules, err := q.ListSchoolSchedules(ctx)
		if err != nil {
			return fmt.Errorf("list schedules before update: %w", err)
		}
		updated, err := q.UpdateSchoolSettings(ctx, db.UpdateSchoolSettingsParams{
			SchoolName:              schoolName,
			Timezone:                input.Timezone,
			AttendanceAutoCloseTime: autoClose,
			UpdatedBy:               actorID,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("update school settings: %w", err))
		}
		if err := q.DeleteSchoolSchedules(ctx); err != nil {
			return fmt.Errorf("delete school schedules: %w", err)
		}
		for _, item := range schedules {
			start, _ := parseTimeOfDay(item.StartTime)
			if err := q.UpsertSchoolSchedule(ctx, db.UpsertSchoolScheduleParams{
				Weekday:              item.Weekday,
				StartTime:            start,
				LateToleranceMinutes: item.LateToleranceMinutes,
			}); err != nil {
				return fmt.Errorf("upsert school schedule: %w", err)
			}
		}
		if err := s.auditData(ctx, q, actorID, "attendance.settings_update", "school_settings", pgtype.UUID{}, settingsAudit(beforeSettings, beforeSchedules), settingsAuditFromInput(updated, schedules), requestID); err != nil {
			return fmt.Errorf("write attendance settings audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return SettingsDetail{}, err
	}
	return s.GetSettings(ctx)
}

func (s *Service) ListHolidays(ctx context.Context, from, to string) ([]Holiday, error) {
	params := db.ListSchoolHolidaysParams{}
	if from != "" {
		date, err := parseDate(from)
		if err != nil {
			return nil, ErrInvalidInput
		}
		params.FromDate = date
	}
	if to != "" {
		date, err := parseDate(to)
		if err != nil {
			return nil, ErrInvalidInput
		}
		params.ToDate = date
	}
	rows, err := s.queries.ListSchoolHolidays(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list school holidays: %w", err)
	}
	items := make([]Holiday, 0, len(rows))
	for _, row := range rows {
		items = append(items, holidayFromRow(row))
	}
	return items, nil
}

func (s *Service) CreateHoliday(ctx context.Context, actor Actor, input HolidayInput, requestID string) (Holiday, error) {
	if !hasRole(actor.Roles, "admin") {
		return Holiday{}, ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return Holiday{}, ErrUnauthorized
	}
	date, err := parseDate(input.Date)
	if err != nil {
		return Holiday{}, ErrInvalidInput
	}
	name, err := requiredText(input.Name, 150)
	if err != nil {
		return Holiday{}, err
	}
	var created db.SchoolHoliday
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		created, err = q.CreateSchoolHoliday(ctx, db.CreateSchoolHolidayParams{
			HolidayDate: date,
			Name:        name,
			CreatedBy:   actorID,
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create school holiday: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "attendance.holiday_create", "school_holiday", pgtype.UUID{}, nil, map[string]string{"date": dateString(date), "name": name}, requestID); err != nil {
			return fmt.Errorf("write holiday audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return Holiday{}, err
	}
	return holidayFromRow(created), nil
}

func (s *Service) DeleteHoliday(ctx context.Context, actor Actor, dateValue, requestID string) error {
	if !hasRole(actor.Roles, "admin") {
		return ErrForbidden
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return ErrUnauthorized
	}
	date, err := parseDate(dateValue)
	if err != nil {
		return ErrInvalidInput
	}
	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		rows, err := q.DeleteSchoolHoliday(ctx, date)
		if err != nil {
			return fmt.Errorf("delete school holiday: %w", err)
		}
		if rows == 0 {
			return ErrNotFound
		}
		if err := s.auditData(ctx, q, actorID, "attendance.holiday_delete", "school_holiday", pgtype.UUID{}, map[string]string{"date": dateValue}, nil, requestID); err != nil {
			return fmt.Errorf("write holiday audit: %w", err)
		}
		return nil
	})
}

type settingsAuditValue struct {
	SchoolName    string          `json:"school_name"`
	Timezone      string          `json:"timezone"`
	AutoCloseTime string          `json:"attendance_auto_close_time"`
	Schedules     []ScheduleInput `json:"schedules"`
}

func settingsAuditFromInput(settings db.SchoolSetting, schedules []ScheduleInput) settingsAuditValue {
	return settingsAuditValue{
		SchoolName:    settings.SchoolName,
		Timezone:      settings.Timezone,
		AutoCloseTime: timeOfDayString(settings.AttendanceAutoCloseTime),
		Schedules:     schedules,
	}
}

func settingsAudit(settings db.SchoolSetting, schedules []db.SchoolSchedule) settingsAuditValue {
	value := settingsAuditValue{
		SchoolName:    settings.SchoolName,
		Timezone:      settings.Timezone,
		AutoCloseTime: timeOfDayString(settings.AttendanceAutoCloseTime),
		Schedules:     make([]ScheduleInput, 0, len(schedules)),
	}
	for _, schedule := range schedules {
		value.Schedules = append(value.Schedules, ScheduleInput{
			Weekday:              schedule.Weekday,
			StartTime:            timeOfDayString(schedule.StartTime),
			LateToleranceMinutes: schedule.LateToleranceMinutes,
		})
	}
	return value
}

func settingsDetail(settings db.SchoolSetting, schedules []db.SchoolSchedule, holidays []db.SchoolHoliday) SettingsDetail {
	detail := SettingsDetail{Settings: settingsFromRow(settings)}
	detail.Schedules = make([]Schedule, 0, len(schedules))
	for _, schedule := range schedules {
		detail.Schedules = append(detail.Schedules, scheduleFromRow(schedule))
	}
	detail.Holidays = make([]Holiday, 0, len(holidays))
	for _, holiday := range holidays {
		detail.Holidays = append(detail.Holidays, holidayFromRow(holiday))
	}
	return detail
}
