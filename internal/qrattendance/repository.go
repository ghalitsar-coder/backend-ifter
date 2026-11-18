package qrattendance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

type PostgresRepository struct {
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{queries: db.New(pool)}
}

func (r *PostgresRepository) CreateSession(ctx context.Context, schoolID, creatorID int64, secret string, input CreateSessionInput, date time.Time, openAt, closeAt string, lateAfter *string) (SessionRecord, error) {
	rotation := int32(30)
	if input.RotationSeconds != nil {
		rotation = int32(*input.RotationSeconds)
	}
	row, err := r.queries.CreateQRSession(ctx, db.CreateQRSessionParams{
		SchoolID: schoolID, CreatedBy: creatorID, TokenQr: secret,
		Tanggal: date, TipeSesi: input.Type, JamBuka: openAt,
		JamTutup: closeAt, BatasTerlambat: lateAfter, RotasiDetik: rotation,
	})
	if err != nil {
		return SessionRecord{}, err
	}
	return SessionRecord{
		Session: Session{
			ID: row.ID, Date: row.Tanggal.Format(dateLayout), Type: row.TipeSesi,
			OpenAt: row.JamBuka, CloseAt: row.JamTutup,
			LateAfter:       optionalString(row.BatasTerlambat),
			RotationSeconds: row.RotasiDetik, Status: row.Status, CreatedAt: row.CreatedAt,
		},
		SchoolID: row.SchoolID, CreatedBy: row.CreatedBy, TokenSecret: row.TokenQr,
	}, nil
}

func (r *PostgresRepository) GetSession(ctx context.Context, id, schoolID int64) (SessionRecord, error) {
	row, err := r.queries.GetQRSession(ctx, db.GetQRSessionParams{ID: id, SchoolID: schoolID})
	if err != nil {
		return SessionRecord{}, err
	}
	return SessionRecord{
		Session: Session{
			ID: row.ID, Date: row.Tanggal.Format(dateLayout), Type: row.TipeSesi,
			OpenAt: row.JamBuka, CloseAt: row.JamTutup,
			LateAfter:       optionalString(row.BatasTerlambat),
			RotationSeconds: row.RotasiDetik, Status: row.Status, CreatedAt: row.CreatedAt,
		},
		SchoolID: row.SchoolID, CreatedBy: row.CreatedBy, TokenSecret: row.TokenQr,
	}, nil
}

func (r *PostgresRepository) CloseSession(ctx context.Context, id, schoolID int64) error {
	_, err := r.queries.CloseQRSession(ctx, db.CloseQRSessionParams{ID: id, SchoolID: schoolID})
	return err
}

func (r *PostgresRepository) FindActiveStaff(ctx context.Context, userID, schoolID int64) (int64, error) {
	return r.queries.FindActiveStaffForSchool(ctx, db.FindActiveStaffForSchoolParams{
		UserID: userID, SchoolID: schoolID,
	})
}

func (r *PostgresRepository) CheckIn(ctx context.Context, staffID, sessionID int64, date, scannedAt time.Time, status string) error {
	_, err := r.queries.RecordStaffCheckIn(ctx, db.RecordStaffCheckInParams{
		StaffID: staffID, SessionID: &sessionID, Tanggal: date,
		ScannedAt: pgtype.Timestamptz{Time: scannedAt, Valid: true}, Status: status,
	})
	return err
}

func (r *PostgresRepository) CheckOut(ctx context.Context, staffID, sessionID int64, date, scannedAt time.Time) error {
	_, err := r.queries.RecordStaffCheckOut(ctx, db.RecordStaffCheckOutParams{
		SessionID: &sessionID, ScannedAt: pgtype.Timestamptz{Time: scannedAt, Valid: true},
		StaffID: staffID, Tanggal: date,
	})
	return err
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
