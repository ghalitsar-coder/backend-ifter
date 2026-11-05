package recap

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

type Repository interface {
	GenerateStudentRecaps(ctx context.Context, schoolID int64, month, year int16) error
	ListStudentRecaps(ctx context.Context, schoolID int64, month, year int16) ([]StudentRecap, error)
	StudentRecapStatus(ctx context.Context, id, schoolID int64) (string, error)
	VerifyStudentRecap(ctx context.Context, id, schoolID, userID int64) (Verification, error)
	GenerateStaffRecaps(ctx context.Context, schoolID int64, month, year int16) error
	ListStaffRecaps(ctx context.Context, schoolID int64, month, year int16) ([]StaffRecap, error)
	StaffRecapStatus(ctx context.Context, id, schoolID int64) (string, error)
	VerifyStaffRecap(ctx context.Context, id, schoolID, userID int64) (Verification, error)
}

type PostgresRepository struct {
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{queries: db.New(pool)}
}

func (r *PostgresRepository) GenerateStudentRecaps(ctx context.Context, schoolID int64, month, year int16) error {
	_, err := r.queries.GenerateStudentAttendanceRecaps(ctx, db.GenerateStudentAttendanceRecapsParams{
		Bulan: month, Tahun: year, SchoolID: schoolID,
	})
	return err
}

func (r *PostgresRepository) ListStudentRecaps(ctx context.Context, schoolID int64, month, year int16) ([]StudentRecap, error) {
	rows, err := r.queries.ListStudentAttendanceRecaps(ctx, db.ListStudentAttendanceRecapsParams{
		SchoolID: schoolID, Bulan: month, Tahun: year,
	})
	if err != nil {
		return nil, err
	}

	items := make([]StudentRecap, 0, len(rows))
	for _, row := range rows {
		items = append(items, StudentRecap{
			ID: row.ID, StudentID: row.StudentID, StudentName: row.StudentName,
			ClassID: row.ClassID, ClassName: row.ClassName, Month: row.Bulan,
			Year: row.Tahun, TotalHadir: row.TotalHadir, TotalSakit: row.TotalSakit,
			TotalIzin: row.TotalIzin, TotalAlpa: row.TotalAlpa,
			AttendanceRate: row.PersentaseHadir, Status: row.StatusKunci,
			VerifiedBy: row.VerifiedBy, CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

func (r *PostgresRepository) StudentRecapStatus(ctx context.Context, id, schoolID int64) (string, error) {
	return r.queries.StudentAttendanceRecapStatus(ctx, db.StudentAttendanceRecapStatusParams{
		ID: id, SchoolID: schoolID,
	})
}

func (r *PostgresRepository) VerifyStudentRecap(ctx context.Context, id, schoolID, userID int64) (Verification, error) {
	row, err := r.queries.VerifyStudentAttendanceRecap(ctx, db.VerifyStudentAttendanceRecapParams{
		ID: id, SchoolID: schoolID, UserID: &userID,
	})
	if err != nil {
		return Verification{}, err
	}
	return Verification{ID: row.ID, Status: row.StatusKunci, VerifiedBy: row.VerifiedBy}, nil
}

func (r *PostgresRepository) GenerateStaffRecaps(ctx context.Context, schoolID int64, month, year int16) error {
	_, err := r.queries.GenerateStaffAttendanceRecaps(ctx, db.GenerateStaffAttendanceRecapsParams{
		Bulan: month, Tahun: year, SchoolID: schoolID,
	})
	return err
}

func (r *PostgresRepository) ListStaffRecaps(ctx context.Context, schoolID int64, month, year int16) ([]StaffRecap, error) {
	rows, err := r.queries.ListStaffAttendanceRecaps(ctx, db.ListStaffAttendanceRecapsParams{
		SchoolID: schoolID, Bulan: month, Tahun: year,
	})
	if err != nil {
		return nil, err
	}
	items := make([]StaffRecap, 0, len(rows))
	for _, row := range rows {
		items = append(items, StaffRecap{
			ID: row.ID, StaffID: row.StaffID, StaffName: row.StaffName, NIP: row.Nip,
			Month: row.Bulan, Year: row.Tahun, TotalWorkdays: row.TotalHariKerja,
			TotalHadir: row.TotalHadir, TotalTerlambat: row.TotalTerlambat,
			TotalIzin: row.TotalIzin, TotalSakit: row.TotalSakit, TotalAlpa: row.TotalAlpa,
			AttendanceRate: row.PersentaseHadir, Status: row.StatusKunci,
			VerifiedBy: row.VerifiedBy, CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

func (r *PostgresRepository) StaffRecapStatus(ctx context.Context, id, schoolID int64) (string, error) {
	return r.queries.StaffAttendanceRecapStatus(ctx, db.StaffAttendanceRecapStatusParams{
		ID: id, SchoolID: schoolID,
	})
}

func (r *PostgresRepository) VerifyStaffRecap(ctx context.Context, id, schoolID, userID int64) (Verification, error) {
	row, err := r.queries.VerifyStaffAttendanceRecap(ctx, db.VerifyStaffAttendanceRecapParams{
		ID: id, SchoolID: schoolID, UserID: &userID,
	})
	if err != nil {
		return Verification{}, err
	}
	return Verification{ID: row.ID, Status: row.StatusKunci, VerifiedBy: row.VerifiedBy}, nil
}
