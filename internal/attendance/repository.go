package attendance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Repository is the persistence contract the service depends on. Tests use a
// fake; production uses PostgresRepository.
type Repository interface {
	StaffIDByUserID(ctx context.Context, userID int64) (int64, error)
	CanRecord(ctx context.Context, staffID, classID, schoolID int64, classSubjectID *int64) (bool, error)
	ClassInSchool(ctx context.Context, classID, schoolID int64) (bool, error)
	CreateSession(ctx context.Context, classID, recordedBy int64, classSubjectID *int64, tanggal time.Time) (Session, error)
	GetSession(ctx context.Context, id, schoolID int64) (Session, error)
	ListAttendances(ctx context.Context, sessionID, classID int64) ([]Attendance, error)
	CountEnrolledStudents(ctx context.Context, classID int64, studentIDs []int64) (int64, error)
	RecordAttendances(ctx context.Context, sessionID int64, attendances []AttendanceInput) error
	LockSession(ctx context.Context, id int64) (Session, error)
	ListClasses(ctx context.Context, schoolID int64) ([]Class, error)
}

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, queries: db.New(pool)}
}

func (r *PostgresRepository) StaffIDByUserID(ctx context.Context, userID int64) (int64, error) {
	return r.queries.StaffIDByUserID(ctx, userID)
}

func (r *PostgresRepository) CanRecord(ctx context.Context, staffID, classID, schoolID int64, classSubjectID *int64) (bool, error) {
	return r.queries.CanRecordAttendance(ctx, db.CanRecordAttendanceParams{
		ClassID:        classID,
		SchoolID:       schoolID,
		ClassSubjectID: classSubjectID,
		StaffID:        staffID,
	})
}

func (r *PostgresRepository) ClassInSchool(ctx context.Context, classID, schoolID int64) (bool, error) {
	return r.queries.ClassInSchool(ctx, db.ClassInSchoolParams{ClassID: classID, SchoolID: schoolID})
}

func (r *PostgresRepository) CreateSession(ctx context.Context, classID, recordedBy int64, classSubjectID *int64, tanggal time.Time) (Session, error) {
	row, err := r.queries.CreateAttendanceSession(ctx, db.CreateAttendanceSessionParams{
		ClassID:        classID,
		ClassSubjectID: classSubjectID,
		RecordedBy:     recordedBy,
		Tanggal:        tanggal,
	})
	if err != nil {
		return Session{}, err
	}
	return Session{
		ID:             row.ID,
		ClassID:        row.ClassID,
		ClassSubjectID: row.ClassSubjectID,
		RecordedBy:     row.RecordedBy,
		Tanggal:        Date(row.Tanggal),
		IsLocked:       row.IsLocked,
		CreatedAt:      row.CreatedAt,
	}, nil
}

func (r *PostgresRepository) GetSession(ctx context.Context, id, schoolID int64) (Session, error) {
	row, err := r.queries.GetAttendanceSession(ctx, db.GetAttendanceSessionParams{ID: id, SchoolID: schoolID})
	if err != nil {
		return Session{}, err
	}
	return toSession(row), nil
}

func (r *PostgresRepository) ListAttendances(ctx context.Context, sessionID, classID int64) ([]Attendance, error) {
	rows, err := r.queries.ListSessionAttendances(ctx, db.ListSessionAttendancesParams{
		SessionID: sessionID,
		ClassID:   classID,
	})
	if err != nil {
		return nil, err
	}
	attendances := make([]Attendance, 0, len(rows))
	for _, row := range rows {
		attendances = append(attendances, Attendance{
			StudentID:  row.StudentID,
			NIS:        row.Nis,
			Nama:       row.Nama,
			Status:     row.Status,
			Keterangan: row.Keterangan,
		})
	}
	return attendances, nil
}

func (r *PostgresRepository) CountEnrolledStudents(ctx context.Context, classID int64, studentIDs []int64) (int64, error) {
	return r.queries.CountEnrolledStudents(ctx, db.CountEnrolledStudentsParams{
		ClassID:    classID,
		StudentIds: studentIDs,
	})
}

// RecordAttendances locks the session row first so a concurrent lock cannot
// slip in between the service's check and the writes.
func (r *PostgresRepository) RecordAttendances(ctx context.Context, sessionID int64, attendances []AttendanceInput) error {
	return database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		queries := db.New(tx)
		locked, err := queries.GetAttendanceSessionLockState(ctx, sessionID)
		if err != nil {
			return err
		}
		if locked {
			return ErrLocked
		}
		for _, entry := range attendances {
			err := queries.UpsertStudentAttendance(ctx, db.UpsertStudentAttendanceParams{
				SessionID:  sessionID,
				StudentID:  entry.StudentID,
				Status:     entry.Status,
				Keterangan: entry.Keterangan,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *PostgresRepository) LockSession(ctx context.Context, id int64) (Session, error) {
	row, err := r.queries.LockAttendanceSession(ctx, id)
	if err != nil {
		return Session{}, err
	}
	return toSession(row), nil
}

func (r *PostgresRepository) ListClasses(ctx context.Context, schoolID int64) ([]Class, error) {
	rows, err := r.queries.ListActiveClasses(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	classes := make([]Class, 0, len(rows))
	for _, row := range rows {
		classes = append(classes, Class{
			ID:        row.ID,
			NamaKelas: row.NamaKelas,
			Tingkat:   int(row.Tingkat),
			WaliKelas: row.WaliKelas,
		})
	}
	return classes, nil
}

func toSession(row db.AttendanceSession) Session {
	return Session{
		ID:             row.ID,
		ClassID:        row.ClassID,
		ClassSubjectID: row.ClassSubjectID,
		RecordedBy:     row.RecordedBy,
		Tanggal:        Date(row.Tanggal),
		IsLocked:       row.IsLocked,
		CreatedAt:      row.CreatedAt,
	}
}
