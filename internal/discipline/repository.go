package discipline

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

type Repository interface {
	StaffIDByUserID(ctx context.Context, userID int64) (int64, error)
	ClassInSchool(ctx context.Context, classID, schoolID int64) (bool, error)
	ListViolationTypes(ctx context.Context, schoolID int64) ([]ViolationType, error)
	GetViolationTypeByID(ctx context.Context, id, schoolID int64) (ViolationType, error)
	RecordViolation(ctx context.Context, schoolID int64, in RecordViolationInput, poin int32, tanggal time.Time) (StudentViolation, error)
	ListViolations(ctx context.Context, schoolID int64, filter ListViolationsFilter) ([]StudentViolation, error)
	GetStudentSummary(ctx context.Context, schoolID, studentID int64) (StudentSummary, error)
	UpdateFollowup(ctx context.Context, schoolID, id int64, followup string) (StudentViolation, error)
	CreateInspectionWithViolations(ctx context.Context, schoolID, inspectorID int64, in CreateInspectionInput, tanggal time.Time, defaultPoints map[int64]int32) (DisciplineInspection, error)
	ListInspections(ctx context.Context, schoolID int64, filter ListInspectionsFilter) ([]DisciplineInspection, error)
	ListClassStudents(ctx context.Context, schoolID, classID int64) ([]ClassStudent, error)
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

func (r *PostgresRepository) ClassInSchool(ctx context.Context, classID, schoolID int64) (bool, error) {
	return r.queries.ClassInSchool(ctx, db.ClassInSchoolParams{ClassID: classID, SchoolID: schoolID})
}

func (r *PostgresRepository) ListViolationTypes(ctx context.Context, schoolID int64) ([]ViolationType, error) {
	rows, err := r.queries.ListViolationTypes(ctx, schoolID)
	if err != nil {
		return nil, err
	}
	res := make([]ViolationType, 0, len(rows))
	for _, row := range rows {
		res = append(res, ViolationType{
			ID:          row.ID,
			SchoolID:    row.SchoolID,
			Kode:        row.Kode,
			Nama:        row.Nama,
			Kategori:    row.Kategori,
			PoinDefault: row.PoinDefault,
			IsActive:    row.IsActive,
			CreatedAt:   row.CreatedAt,
		})
	}
	return res, nil
}

func (r *PostgresRepository) GetViolationTypeByID(ctx context.Context, id, schoolID int64) (ViolationType, error) {
	row, err := r.queries.GetViolationTypeByID(ctx, db.GetViolationTypeByIDParams{
		ID:       id,
		SchoolID: schoolID,
	})
	if err != nil {
		return ViolationType{}, err
	}
	return ViolationType{
		ID:          row.ID,
		SchoolID:    row.SchoolID,
		Kode:        row.Kode,
		Nama:        row.Nama,
		Kategori:    row.Kategori,
		PoinDefault: row.PoinDefault,
		IsActive:    row.IsActive,
		CreatedAt:   row.CreatedAt,
	}, nil
}

func (r *PostgresRepository) RecordViolation(ctx context.Context, schoolID int64, in RecordViolationInput, poin int32, tanggal time.Time) (StudentViolation, error) {
	row, err := r.queries.RecordStudentViolation(ctx, db.RecordStudentViolationParams{
		SchoolID:        schoolID,
		InspectionID:    in.InspectionID,
		StudentID:       in.StudentID,
		ViolationTypeID: in.ViolationTypeID,
		Poin:            poin,
		Keterangan:      in.Keterangan,
		TindakLanjut:    in.TindakLanjut,
		Tanggal:         tanggal,
	})
	if err != nil {
		return StudentViolation{}, err
	}

	// Fetch detail with names
	rows, err := r.queries.ListStudentViolations(ctx, db.ListStudentViolationsParams{
		SchoolID:  schoolID,
		StudentID: &row.StudentID,
	})
	if err != nil || len(rows) == 0 {
		return StudentViolation{
			ID:              row.ID,
			SchoolID:        row.SchoolID,
			InspectionID:    row.InspectionID,
			StudentID:       row.StudentID,
			ViolationTypeID: row.ViolationTypeID,
			Poin:            row.Poin,
			Keterangan:      row.Keterangan,
			TindakLanjut:    row.TindakLanjut,
			Tanggal:         Date(row.Tanggal),
			CreatedAt:       row.CreatedAt,
		}, nil
	}

	// Find the exact inserted record in list
	for _, it := range rows {
		if it.ID == row.ID {
			return StudentViolation{
				ID:                it.ID,
				SchoolID:          it.SchoolID,
				InspectionID:      it.InspectionID,
				StudentID:         it.StudentID,
				ViolationTypeID:   it.ViolationTypeID,
				Poin:              it.Poin,
				Keterangan:        it.Keterangan,
				TindakLanjut:      it.TindakLanjut,
				Tanggal:           Date(it.Tanggal),
				CreatedAt:         it.CreatedAt,
				NIS:               it.Nis,
				StudentName:       it.StudentName,
				NamaKelas:         it.NamaKelas,
				ViolationCode:     it.ViolationCode,
				ViolationName:     it.ViolationName,
				ViolationCategory: it.ViolationCategory,
			}, nil
		}
	}

	return StudentViolation{
		ID:              row.ID,
		SchoolID:        row.SchoolID,
		InspectionID:    row.InspectionID,
		StudentID:       row.StudentID,
		ViolationTypeID: row.ViolationTypeID,
		Poin:            row.Poin,
		Keterangan:      row.Keterangan,
		TindakLanjut:    row.TindakLanjut,
		Tanggal:         Date(row.Tanggal),
		CreatedAt:       row.CreatedAt,
	}, nil
}

func (r *PostgresRepository) ListViolations(ctx context.Context, schoolID int64, filter ListViolationsFilter) ([]StudentViolation, error) {
	rows, err := r.queries.ListStudentViolations(ctx, db.ListStudentViolationsParams{
		SchoolID:  schoolID,
		StudentID: filter.StudentID,
		ClassID:   filter.ClassID,
		Kategori:  filter.Kategori,
	})
	if err != nil {
		return nil, err
	}
	res := make([]StudentViolation, 0, len(rows))
	for _, row := range rows {
		res = append(res, StudentViolation{
			ID:                row.ID,
			SchoolID:          row.SchoolID,
			InspectionID:      row.InspectionID,
			StudentID:         row.StudentID,
			ViolationTypeID:   row.ViolationTypeID,
			Poin:              row.Poin,
			Keterangan:        row.Keterangan,
			TindakLanjut:      row.TindakLanjut,
			Tanggal:           Date(row.Tanggal),
			CreatedAt:         row.CreatedAt,
			NIS:               row.Nis,
			StudentName:       row.StudentName,
			NamaKelas:         row.NamaKelas,
			ViolationCode:     row.ViolationCode,
			ViolationName:     row.ViolationName,
			ViolationCategory: row.ViolationCategory,
		})
	}
	return res, nil
}

func (r *PostgresRepository) GetStudentSummary(ctx context.Context, schoolID, studentID int64) (StudentSummary, error) {
	row, err := r.queries.GetStudentPointsSummary(ctx, db.GetStudentPointsSummaryParams{
		StudentID: studentID,
		SchoolID:  schoolID,
	})
	if err != nil {
		return StudentSummary{}, err
	}
	return StudentSummary{
		StudentID:      studentID,
		TotalPoints:    row.TotalPoints,
		TotalIncidents: row.TotalIncidents,
		StatusSP:       CalculateSPStatus(row.TotalPoints),
	}, nil
}

func (r *PostgresRepository) UpdateFollowup(ctx context.Context, schoolID, id int64, followup string) (StudentViolation, error) {
	row, err := r.queries.UpdateStudentViolationFollowup(ctx, db.UpdateStudentViolationFollowupParams{
		TindakLanjut: &followup,
		ID:           id,
		SchoolID:     schoolID,
	})
	if err != nil {
		return StudentViolation{}, err
	}
	return StudentViolation{
		ID:              row.ID,
		SchoolID:        row.SchoolID,
		InspectionID:    row.InspectionID,
		StudentID:       row.StudentID,
		ViolationTypeID: row.ViolationTypeID,
		Poin:            row.Poin,
		Keterangan:      row.Keterangan,
		TindakLanjut:    row.TindakLanjut,
		Tanggal:         Date(row.Tanggal),
		CreatedAt:       row.CreatedAt,
	}, nil
}

func (r *PostgresRepository) CreateInspectionWithViolations(ctx context.Context, schoolID, inspectorID int64, in CreateInspectionInput, tanggal time.Time, defaultPoints map[int64]int32) (DisciplineInspection, error) {
	var inspection DisciplineInspection

	err := database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		qtx := r.queries.WithTx(tx)

		ins, err := qtx.CreateDisciplineInspection(ctx, db.CreateDisciplineInspectionParams{
			SchoolID:        schoolID,
			ClassID:         in.ClassID,
			InspectorID:     inspectorID,
			Tanggal:         tanggal,
			JumlahDiperiksa: in.JumlahDiperiksa,
			Catatan:         in.Catatan,
		})
		if err != nil {
			return err
		}

		for _, item := range in.Violations {
			poin := defaultPoints[item.ViolationTypeID]
			if item.Poin != nil {
				poin = *item.Poin
			}
			_, err := qtx.RecordStudentViolation(ctx, db.RecordStudentViolationParams{
				SchoolID:        schoolID,
				InspectionID:    &ins.ID,
				StudentID:       item.StudentID,
				ViolationTypeID: item.ViolationTypeID,
				Poin:            poin,
				Keterangan:      item.Keterangan,
				TindakLanjut:    nil,
				Tanggal:         tanggal,
			})
			if err != nil {
				return err
			}
		}

		inspection = DisciplineInspection{
			ID:              ins.ID,
			SchoolID:        ins.SchoolID,
			ClassID:         ins.ClassID,
			InspectorID:     ins.InspectorID,
			Tanggal:         Date(ins.Tanggal),
			JumlahDiperiksa: ins.JumlahDiperiksa,
			Catatan:         ins.Catatan,
			CreatedAt:       ins.CreatedAt,
		}
		return nil
	})

	if err != nil {
		return DisciplineInspection{}, err
	}

	// Fetch detail with inspector and class name
	detail, err := r.queries.GetDisciplineInspection(ctx, db.GetDisciplineInspectionParams{
		ID:       inspection.ID,
		SchoolID: schoolID,
	})
	if err == nil {
		inspection.NamaKelas = detail.NamaKelas
		inspection.InspectorName = detail.InspectorName
	}

	return inspection, nil
}

func (r *PostgresRepository) ListInspections(ctx context.Context, schoolID int64, filter ListInspectionsFilter) ([]DisciplineInspection, error) {
	var pgDate pgtype.Date
	if filter.Tanggal != nil {
		pgDate = pgtype.Date{
			Time:  filter.Tanggal.Time(),
			Valid: true,
		}
	}
	rows, err := r.queries.ListDisciplineInspections(ctx, db.ListDisciplineInspectionsParams{
		SchoolID: schoolID,
		ClassID:  filter.ClassID,
		Tanggal:  pgDate,
	})
	if err != nil {
		return nil, err
	}
	res := make([]DisciplineInspection, 0, len(rows))
	for _, row := range rows {
		res = append(res, DisciplineInspection{
			ID:              row.ID,
			SchoolID:        row.SchoolID,
			ClassID:         row.ClassID,
			InspectorID:     row.InspectorID,
			Tanggal:         Date(row.Tanggal),
			JumlahDiperiksa: row.JumlahDiperiksa,
			Catatan:         row.Catatan,
			CreatedAt:       row.CreatedAt,
			NamaKelas:       row.NamaKelas,
			InspectorName:   row.InspectorName,
		})
	}
	return res, nil
}

func (r *PostgresRepository) ListClassStudents(ctx context.Context, schoolID, classID int64) ([]ClassStudent, error) {
	rows, err := r.queries.ListClassStudentsForInspection(ctx, db.ListClassStudentsForInspectionParams{
		ClassID:  classID,
		SchoolID: schoolID,
	})
	if err != nil {
		return nil, err
	}
	res := make([]ClassStudent, 0, len(rows))
	for _, row := range rows {
		res = append(res, ClassStudent{
			ID:           row.ID,
			NIS:          row.Nis,
			Nama:         row.Nama,
			JenisKelamin: row.JenisKelamin,
			NamaKelas:    row.NamaKelas,
		})
	}
	return res, nil
}
