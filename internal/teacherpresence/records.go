package teacherpresence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// CheckIn records the actor's own presence for the school day of check_time.
// An outside-geofence fix is stored (within_geofence=false) rather than
// rejected outright, mirroring the addendum's 201 + verdict shape.
func (s *Service) CheckIn(ctx context.Context, actor Actor, input CheckInput) (CheckInResult, error) {
	teacherID, err := s.teacherIDForActor(ctx, actor)
	if err != nil {
		return CheckInResult{}, err
	}
	if err := validateFix(input.Latitude, input.Longitude, input.Accuracy); err != nil {
		return CheckInResult{}, err
	}
	notes, err := cleanNotes(input.Notes)
	if err != nil {
		return CheckInResult{}, err
	}
	checkAt, err := resolveCheckTime(input.CheckTime)
	if err != nil {
		return CheckInResult{}, err
	}
	loc, err := s.location(ctx)
	if err != nil {
		return CheckInResult{}, err
	}
	date := dateOnly(checkAt.In(loc))
	if date.Time.After(todayIn(loc)) {
		return CheckInResult{}, ErrInvalidInput
	}
	geofence, err := s.geofence(ctx)
	if err != nil {
		return CheckInResult{}, err
	}
	distance := HaversineMeters(input.Latitude, input.Longitude, geofence.Latitude, geofence.Longitude)
	within := withinGeofence(geofence.IsEnabled, distance, geofence.RadiusMeters)

	if _, err := s.queries.GetTeacherPresenceByDate(ctx, db.GetTeacherPresenceByDateParams{
		TeacherID:    teacherID,
		PresenceDate: date,
	}); err == nil {
		return CheckInResult{}, ErrConflict
	} else if !database.IsNotFound(err) {
		return CheckInResult{}, fmt.Errorf("get teacher presence: %w", err)
	}

	created, err := s.queries.CreateTeacherPresence(ctx, db.CreateTeacherPresenceParams{
		TeacherID:             teacherID,
		PresenceDate:          date,
		CheckInAt:             pgtype.Timestamptz{Time: checkAt, Valid: true},
		CheckInLatitude:       input.Latitude,
		CheckInLongitude:      input.Longitude,
		CheckInAccuracyMeters: input.Accuracy,
		CheckInWithinGeofence: within,
		CheckInDistanceMeters: distance,
		Notes:                 notes,
	})
	if err != nil {
		return CheckInResult{}, mapWriteError(fmt.Errorf("create teacher presence: %w", err))
	}
	return checkInResult(created, within, distance), nil
}

// CheckOut completes the actor's own presence row for the school day of
// check_time. Missing check-in and double check-out are conflicts (409).
func (s *Service) CheckOut(ctx context.Context, actor Actor, input CheckInput) (CheckOutResult, error) {
	teacherID, err := s.teacherIDForActor(ctx, actor)
	if err != nil {
		return CheckOutResult{}, err
	}
	if err := validateFix(input.Latitude, input.Longitude, input.Accuracy); err != nil {
		return CheckOutResult{}, err
	}
	notes, err := cleanNotes(input.Notes)
	if err != nil {
		return CheckOutResult{}, err
	}
	checkAt, err := resolveCheckTime(input.CheckTime)
	if err != nil {
		return CheckOutResult{}, err
	}
	loc, err := s.location(ctx)
	if err != nil {
		return CheckOutResult{}, err
	}
	date := dateOnly(checkAt.In(loc))
	geofence, err := s.geofence(ctx)
	if err != nil {
		return CheckOutResult{}, err
	}

	existing, err := s.queries.GetTeacherPresenceByDate(ctx, db.GetTeacherPresenceByDateParams{
		TeacherID:    teacherID,
		PresenceDate: date,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return CheckOutResult{}, ErrConflict
		}
		return CheckOutResult{}, fmt.Errorf("get teacher presence: %w", err)
	}
	if existing.CheckOutAt.Valid {
		return CheckOutResult{}, ErrConflict
	}
	checkInAt := timestamp(existing.CheckInAt)
	if checkAt.Before(checkInAt) {
		return CheckOutResult{}, ErrInvalidInput
	}

	distance := HaversineMeters(input.Latitude, input.Longitude, geofence.Latitude, geofence.Longitude)
	within := withinGeofence(geofence.IsEnabled, distance, geofence.RadiusMeters)
	updated, err := s.queries.SetTeacherPresenceCheckOut(ctx, db.SetTeacherPresenceCheckOutParams{
		ID:                     existing.ID,
		CheckOutAt:             pgtype.Timestamptz{Time: checkAt, Valid: true},
		CheckOutLatitude:       pgtype.Float8{Float64: input.Latitude, Valid: true},
		CheckOutLongitude:      pgtype.Float8{Float64: input.Longitude, Valid: true},
		CheckOutAccuracyMeters: pgtype.Float8{Float64: input.Accuracy, Valid: true},
		CheckOutWithinGeofence: pgtype.Bool{Bool: within, Valid: true},
		CheckOutDistanceMeters: pgtype.Float8{Float64: distance, Valid: true},
		Notes:                  notes,
	})
	if err != nil {
		return CheckOutResult{}, mapWriteError(fmt.Errorf("set teacher presence check-out: %w", err))
	}
	return checkOutResult(updated, within, distance), nil
}

// Today returns the actor's presence for the current school day, with null
// verdict fields when no row exists yet.
func (s *Service) Today(ctx context.Context, actor Actor) (TodayResult, error) {
	teacherID, err := s.teacherIDForActor(ctx, actor)
	if err != nil {
		return TodayResult{}, err
	}
	loc, err := s.location(ctx)
	if err != nil {
		return TodayResult{}, err
	}
	date := pgtype.Date{Time: todayIn(loc), Valid: true}
	row, err := s.queries.GetTeacherPresenceByDate(ctx, db.GetTeacherPresenceByDateParams{
		TeacherID:    teacherID,
		PresenceDate: date,
	})
	if err != nil {
		if database.IsNotFound(err) {
			return TodayResult{
				TeacherID:    uuidString(teacherID),
				PresenceDate: dateString(date),
				CheckInAt:    nil,
				CheckOutAt:   nil,
			}, nil
		}
		return TodayResult{}, fmt.Errorf("get teacher presence: %w", err)
	}
	return TodayResult{
		TeacherID:      uuidString(row.TeacherID),
		PresenceDate:   dateString(row.PresenceDate),
		CheckInAt:      timestampPtr(row.CheckInAt),
		CheckOutAt:     timestampPtr(row.CheckOutAt),
		WithinGeofence: &row.CheckInWithinGeofence,
		DistanceMeters: float8Ptr(pgtype.Float8{Float64: row.CheckInDistanceMeters, Valid: true}),
	}, nil
}

// History lists the actor's own presence rows, newest school day first.
func (s *Service) History(ctx context.Context, actor Actor, filter HistoryFilter) (HistoryResult, error) {
	teacherID, err := s.teacherIDForActor(ctx, actor)
	if err != nil {
		return HistoryResult{}, err
	}
	params := db.ListTeacherPresencesParams{TeacherID: teacherID}
	params.PageLimit, params.PageOffset = normalizePage(filter.Limit, filter.Offset)
	if filter.From != "" {
		from, err := parseDate(filter.From)
		if err != nil {
			return HistoryResult{}, ErrInvalidInput
		}
		params.FromDate = from
	}
	if filter.To != "" {
		to, err := parseDate(filter.To)
		if err != nil {
			return HistoryResult{}, ErrInvalidInput
		}
		params.ToDate = to
	}
	if params.FromDate.Valid && params.ToDate.Valid && params.ToDate.Time.Before(params.FromDate.Time) {
		return HistoryResult{}, ErrInvalidInput
	}

	total, err := s.queries.CountTeacherPresences(ctx, db.CountTeacherPresencesParams{
		TeacherID: teacherID,
		FromDate:  params.FromDate,
		ToDate:    params.ToDate,
	})
	if err != nil {
		return HistoryResult{}, fmt.Errorf("count teacher presences: %w", err)
	}
	rows, err := s.queries.ListTeacherPresences(ctx, params)
	if err != nil {
		return HistoryResult{}, fmt.Errorf("list teacher presences: %w", err)
	}
	data := make([]HistoryEntry, 0, len(rows))
	for _, row := range rows {
		data = append(data, HistoryEntry{
			ID:              uuidString(row.ID),
			TeacherID:       uuidString(row.TeacherID),
			PresenceDate:    dateString(row.PresenceDate),
			CheckInAt:       timestamp(row.CheckInAt),
			CheckOutAt:      timestampPtr(row.CheckOutAt),
			DurationMinutes: durationMinutesBetween(timestamp(row.CheckInAt), timestamp(row.CheckOutAt)),
			WithinGeofence:  row.CheckInWithinGeofence,
			DistanceMeters:  round2(row.CheckInDistanceMeters),
		})
	}
	return HistoryResult{
		Data:   data,
		Total:  total,
		Limit:  params.PageLimit,
		Offset: params.PageOffset,
	}, nil
}

func checkInResult(row db.TeacherPresence, within bool, distance float64) CheckInResult {
	return CheckInResult{
		ID:             uuidString(row.ID),
		TeacherID:      uuidString(row.TeacherID),
		PresenceDate:   dateString(row.PresenceDate),
		CheckInAt:      timestamp(row.CheckInAt),
		WithinGeofence: within,
		DistanceMeters: round2(distance),
		Message:        presenceMessage("check-in", within),
	}
}

func checkOutResult(row db.TeacherPresence, within bool, distance float64) CheckOutResult {
	checkInAt := timestamp(row.CheckInAt)
	checkOutAt := timestamp(row.CheckOutAt)
	return CheckOutResult{
		ID:                     uuidString(row.ID),
		TeacherID:              uuidString(row.TeacherID),
		PresenceDate:           dateString(row.PresenceDate),
		CheckInAt:              checkInAt,
		CheckOutAt:             checkOutAt,
		DurationMinutes:        durationMinutesBetween(checkInAt, checkOutAt),
		CheckOutWithinGeofence: within,
		CheckOutDistanceMeters: round2(distance),
		Message:                presenceMessage("check-out", within),
	}
}

func presenceMessage(action string, within bool) string {
	if within {
		return action + " recorded within the school geofence"
	}
	return action + " recorded outside the school geofence; an administrator can review the deviation"
}
