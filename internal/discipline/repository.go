package discipline

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

type Repository interface {
	StaffIDByUserID(ctx context.Context, userID int64) (int64, error)
	ClassInSchool(ctx context.Context, classID, schoolID int64) (bool, error)
	StudentInSchool(ctx context.Context, studentID, schoolID int64) (bool, error)

	ListViolationTypes(ctx context.Context, schoolID int64, includeInactive bool) ([]ViolationType, error)
	GetViolationTypeByID(ctx context.Context, id, schoolID int64) (ViolationType, error)
	CreateViolationType(ctx context.Context, schoolID int64, in UpsertViolationTypeInput, isActive bool) (ViolationType, error)
	UpdateViolationType(ctx context.Context, schoolID, id int64, in UpsertViolationTypeInput, isActive bool) (ViolationType, error)
	DeleteViolationType(ctx context.Context, schoolID, id int64) error

	RecordViolation(ctx context.Context, schoolID int64, in RecordViolationInput, poin int32, tanggal time.Time) (StudentViolation, error)
	ListViolations(ctx context.Context, schoolID int64, filter ListViolationsFilter) ([]StudentViolation, error)
	UpdateViolation(ctx context.Context, schoolID, id int64, in UpdateViolationInput, poin int32, tanggal time.Time) (StudentViolation, error)
	UpdateFollowup(ctx context.Context, schoolID, id int64, followup string) (StudentViolation, error)
	DeleteViolation(ctx context.Context, schoolID, id int64) error

	GetStudentSummary(ctx context.Context, schoolID, studentID int64) (StudentSummary, error)
	ListSchoolStudents(ctx context.Context, schoolID int64, classID *int64) ([]SchoolStudent, error)
	ListWatchlist(ctx context.Context, schoolID, minPoints int64) ([]WatchlistStudent, error)

	CreateInspectionWithViolations(ctx context.Context, schoolID, inspectorID int64, in CreateInspectionInput, tanggal time.Time, defaultPoints map[int64]int32) (DisciplineInspection, error)
	ListInspections(ctx context.Context, schoolID int64, filter ListInspectionsFilter) ([]DisciplineInspection, error)
	UpdateInspection(ctx context.Context, schoolID, id int64, in UpdateInspectionInput, tanggal time.Time, defaultPoints map[int64]int32) (DisciplineInspection, error)
	DeleteInspection(ctx context.Context, schoolID, id int64) error
	ListClassStudents(ctx context.Context, schoolID, classID int64) ([]ClassStudent, error)

	ListCounselingRecords(ctx context.Context, schoolID int64, filter ListCounselingFilter) ([]CounselingRecord, error)
	CreateCounselingRecord(ctx context.Context, schoolID int64, counselorID *int64, in CreateCounselingInput, tanggal time.Time) (CounselingRecord, error)
	UpdateCounselingRecord(ctx context.Context, schoolID, id int64, in UpdateCounselingInput, tanggal time.Time) (CounselingRecord, error)
	DeleteCounselingRecord(ctx context.Context, schoolID, id int64) error

	ListParentSummons(ctx context.Context, schoolID int64, filter ListSummonsFilter) ([]ParentSummons, error)
	CreateParentSummons(ctx context.Context, schoolID int64, createdBy *int64, in CreateParentSummonsInput, tanggalSurat, jadwal time.Time) (ParentSummons, error)
	UpdateParentSummons(ctx context.Context, schoolID, id int64, in UpdateParentSummonsInput, tanggalSurat, jadwal time.Time) (ParentSummons, error)
	UpdateParentSummonsStatus(ctx context.Context, schoolID, id int64, status string, catatanHasil *string) (ParentSummons, error)
	DeleteParentSummons(ctx context.Context, schoolID, id int64) error
}

type PostgresRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, queries: db.New(pool)}
}

func mapPGError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503") {
		return ErrConflict
	}
	return err
}

func (r *PostgresRepository) StaffIDByUserID(ctx context.Context, userID int64) (int64, error) {
	return r.queries.StaffIDByUserID(ctx, userID)
}

func (r *PostgresRepository) ClassInSchool(ctx context.Context, classID, schoolID int64) (bool, error) {
	return r.queries.ClassInSchool(ctx, db.ClassInSchoolParams{ClassID: classID, SchoolID: schoolID})
}

func (r *PostgresRepository) StudentInSchool(ctx context.Context, studentID, schoolID int64) (bool, error) {
	return r.queries.StudentInSchool(ctx, db.StudentInSchoolParams{StudentID: studentID, SchoolID: schoolID})
}

func mapViolationType(row db.ViolationType) ViolationType {
	return ViolationType{
		ID:          row.ID,
		SchoolID:    row.SchoolID,
		Kode:        row.Kode,
		Nama:        row.Nama,
		Kategori:    row.Kategori,
		PoinDefault: row.PoinDefault,
		IsActive:    row.IsActive,
		CreatedAt:   row.CreatedAt,
	}
}

func (r *PostgresRepository) ListViolationTypes(ctx context.Context, schoolID int64, includeInactive bool) ([]ViolationType, error) {
	rows, err := r.queries.ListViolationTypes(ctx, db.ListViolationTypesParams{
		SchoolID:        schoolID,
		IncludeInactive: includeInactive,
	})
	if err != nil {
		return nil, err
	}
	res := make([]ViolationType, 0, len(rows))
	for _, row := range rows {
		res = append(res, mapViolationType(row))
	}
	return res, nil
}

func (r *PostgresRepository) GetViolationTypeByID(ctx context.Context, id, schoolID int64) (ViolationType, error) {
	row, err := r.queries.GetViolationTypeByID(ctx, db.GetViolationTypeByIDParams{
		ID:       id,
		SchoolID: schoolID,
	})
	if err != nil {
		return ViolationType{}, mapPGError(err)
	}
	return mapViolationType(row), nil
}

func (r *PostgresRepository) CreateViolationType(ctx context.Context, schoolID int64, in UpsertViolationTypeInput, isActive bool) (ViolationType, error) {
	row, err := r.queries.CreateViolationType(ctx, db.CreateViolationTypeParams{
		SchoolID:    schoolID,
		Kode:        in.Kode,
		Nama:        in.Nama,
		Kategori:    in.Kategori,
		PoinDefault: in.PoinDefault,
		IsActive:    isActive,
	})
	if err != nil {
		return ViolationType{}, mapPGError(err)
	}
	return mapViolationType(row), nil
}

func (r *PostgresRepository) UpdateViolationType(ctx context.Context, schoolID, id int64, in UpsertViolationTypeInput, isActive bool) (ViolationType, error) {
	row, err := r.queries.UpdateViolationType(ctx, db.UpdateViolationTypeParams{
		Kode:        in.Kode,
		Nama:        in.Nama,
		Kategori:    in.Kategori,
		PoinDefault: in.PoinDefault,
		IsActive:    isActive,
		ID:          id,
		SchoolID:    schoolID,
	})
	if err != nil {
		return ViolationType{}, mapPGError(err)
	}
	return mapViolationType(row), nil
}

func (r *PostgresRepository) DeleteViolationType(ctx context.Context, schoolID, id int64) error {
	count, err := r.queries.CountStudentViolationsByType(ctx, db.CountStudentViolationsByTypeParams{
		ViolationTypeID: id,
		SchoolID:        schoolID,
	})
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrConflict
	}
	affected, err := r.queries.DeleteViolationType(ctx, db.DeleteViolationTypeParams{
		ID:       id,
		SchoolID: schoolID,
	})
	if err != nil {
		return mapPGError(err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) getViolationDetail(ctx context.Context, schoolID, id int64, fallback db.StudentViolation) (StudentViolation, error) {
	it, err := r.queries.GetStudentViolationByID(ctx, db.GetStudentViolationByIDParams{
		ID:       id,
		SchoolID: schoolID,
	})
	if err != nil {
		return StudentViolation{
			ID:              fallback.ID,
			SchoolID:        fallback.SchoolID,
			InspectionID:    fallback.InspectionID,
			StudentID:       fallback.StudentID,
			ViolationTypeID: fallback.ViolationTypeID,
			Poin:            fallback.Poin,
			Keterangan:      fallback.Keterangan,
			TindakLanjut:    fallback.TindakLanjut,
			Tanggal:         Date(fallback.Tanggal),
			CreatedAt:       fallback.CreatedAt,
		}, nil
	}
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
		return StudentViolation{}, mapPGError(err)
	}
	return r.getViolationDetail(ctx, schoolID, row.ID, row)
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

func (r *PostgresRepository) UpdateViolation(ctx context.Context, schoolID, id int64, in UpdateViolationInput, poin int32, tanggal time.Time) (StudentViolation, error) {
	row, err := r.queries.UpdateStudentViolation(ctx, db.UpdateStudentViolationParams{
		ViolationTypeID: in.ViolationTypeID,
		Poin:            poin,
		Keterangan:      in.Keterangan,
		TindakLanjut:    in.TindakLanjut,
		Tanggal:         tanggal,
		ID:              id,
		SchoolID:        schoolID,
	})
	if err != nil {
		return StudentViolation{}, mapPGError(err)
	}
	return r.getViolationDetail(ctx, schoolID, row.ID, row)
}

func (r *PostgresRepository) UpdateFollowup(ctx context.Context, schoolID, id int64, followup string) (StudentViolation, error) {
	row, err := r.queries.UpdateStudentViolationFollowup(ctx, db.UpdateStudentViolationFollowupParams{
		TindakLanjut: &followup,
		ID:           id,
		SchoolID:     schoolID,
	})
	if err != nil {
		return StudentViolation{}, mapPGError(err)
	}
	return r.getViolationDetail(ctx, schoolID, row.ID, row)
}

func (r *PostgresRepository) DeleteViolation(ctx context.Context, schoolID, id int64) error {
	affected, err := r.queries.DeleteStudentViolation(ctx, db.DeleteStudentViolationParams{
		ID:       id,
		SchoolID: schoolID,
	})
	if err != nil {
		return mapPGError(err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
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

func (r *PostgresRepository) ListSchoolStudents(ctx context.Context, schoolID int64, classID *int64) ([]SchoolStudent, error) {
	rows, err := r.queries.ListSchoolStudents(ctx, db.ListSchoolStudentsParams{
		SchoolID: schoolID,
		ClassID:  classID,
	})
	if err != nil {
		return nil, err
	}
	res := make([]SchoolStudent, 0, len(rows))
	for _, row := range rows {
		res = append(res, SchoolStudent{
			ID:           row.ID,
			NIS:          row.Nis,
			Nama:         row.Nama,
			JenisKelamin: row.JenisKelamin,
			NoHpOrtu:     row.NoHpOrtu,
			ClassID:      row.ClassID,
			NamaKelas:    row.NamaKelas,
			Tingkat:      row.Tingkat,
		})
	}
	return res, nil
}

func (r *PostgresRepository) ListWatchlist(ctx context.Context, schoolID, minPoints int64) ([]WatchlistStudent, error) {
	rows, err := r.queries.ListStudentsWatchlist(ctx, db.ListStudentsWatchlistParams{
		SchoolID:  schoolID,
		MinPoints: minPoints,
	})
	if err != nil {
		return nil, err
	}
	res := make([]WatchlistStudent, 0, len(rows))
	for _, row := range rows {
		res = append(res, WatchlistStudent{
			StudentID:         row.StudentID,
			NIS:               row.Nis,
			StudentName:       row.StudentName,
			NoHpOrtu:          row.NoHpOrtu,
			ClassID:           row.ClassID,
			NamaKelas:         row.NamaKelas,
			TotalPoints:       row.TotalPoints,
			TotalIncidents:    row.TotalIncidents,
			LastViolationDate: Date(row.LastViolationDate),
			StatusSP:          CalculateSPStatus(row.TotalPoints),
		})
	}
	return res, nil
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

		// Replace any prior violations on this inspection date/class when updating via conflict.
		if err := qtx.DeleteStudentViolationsByInspection(ctx, db.DeleteStudentViolationsByInspectionParams{
			InspectionID: &ins.ID,
			SchoolID:     schoolID,
		}); err != nil {
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
			Violations:      []StudentViolation{},
		}
		return nil
	})

	if err != nil {
		return DisciplineInspection{}, mapPGError(err)
	}

	detail, err := r.queries.GetDisciplineInspection(ctx, db.GetDisciplineInspectionParams{
		ID:       inspection.ID,
		SchoolID: schoolID,
	})
	if err == nil {
		inspection.NamaKelas = detail.NamaKelas
		inspection.InspectorName = detail.InspectorName
	}
	list := []DisciplineInspection{inspection}
	r.attachInspectionViolations(ctx, schoolID, list)

	return list[0], nil
}

func (r *PostgresRepository) attachInspectionViolations(ctx context.Context, schoolID int64, inspections []DisciplineInspection) {
	if len(inspections) == 0 {
		return
	}
	ids := make([]int64, 0, len(inspections))
	idxMap := make(map[int64]int, len(inspections))
	for i := range inspections {
		inspections[i].Violations = []StudentViolation{}
		ids = append(ids, inspections[i].ID)
		idxMap[inspections[i].ID] = i
	}
	rows, err := r.queries.ListViolationsByInspectionIDs(ctx, db.ListViolationsByInspectionIDsParams{
		SchoolID:      schoolID,
		InspectionIds: ids,
	})
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.InspectionID == nil {
			continue
		}
		idx, ok := idxMap[*row.InspectionID]
		if !ok {
			continue
		}
		inspections[idx].Violations = append(inspections[idx].Violations, StudentViolation{
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
			Violations:      []StudentViolation{},
		})
	}
	r.attachInspectionViolations(ctx, schoolID, res)
	return res, nil
}

func (r *PostgresRepository) UpdateInspection(ctx context.Context, schoolID, id int64, in UpdateInspectionInput, tanggal time.Time, defaultPoints map[int64]int32) (DisciplineInspection, error) {
	var ins DisciplineInspection
	err := database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		qtx := r.queries.WithTx(tx)
		row, err := qtx.UpdateDisciplineInspection(ctx, db.UpdateDisciplineInspectionParams{
			Tanggal:         tanggal,
			JumlahDiperiksa: in.JumlahDiperiksa,
			Catatan:         in.Catatan,
			ID:              id,
			SchoolID:        schoolID,
		})
		if err != nil {
			return err
		}

		if in.Violations != nil {
			if err := qtx.DeleteStudentViolationsByInspection(ctx, db.DeleteStudentViolationsByInspectionParams{
				InspectionID: &row.ID,
				SchoolID:     schoolID,
			}); err != nil {
				return err
			}
			for _, item := range in.Violations {
				poin := defaultPoints[item.ViolationTypeID]
				if item.Poin != nil {
					poin = *item.Poin
				}
				if _, err := qtx.RecordStudentViolation(ctx, db.RecordStudentViolationParams{
					SchoolID:        schoolID,
					InspectionID:    &row.ID,
					StudentID:       item.StudentID,
					ViolationTypeID: item.ViolationTypeID,
					Poin:            poin,
					Keterangan:      item.Keterangan,
					TindakLanjut:    nil,
					Tanggal:         tanggal,
				}); err != nil {
					return err
				}
			}
		}

		ins = DisciplineInspection{
			ID:              row.ID,
			SchoolID:        row.SchoolID,
			ClassID:         row.ClassID,
			InspectorID:     row.InspectorID,
			Tanggal:         Date(row.Tanggal),
			JumlahDiperiksa: row.JumlahDiperiksa,
			Catatan:         row.Catatan,
			CreatedAt:       row.CreatedAt,
			Violations:      []StudentViolation{},
		}
		return nil
	})
	if err != nil {
		return DisciplineInspection{}, mapPGError(err)
	}
	if detail, err := r.queries.GetDisciplineInspection(ctx, db.GetDisciplineInspectionParams{ID: ins.ID, SchoolID: schoolID}); err == nil {
		ins.NamaKelas = detail.NamaKelas
		ins.InspectorName = detail.InspectorName
	}
	list := []DisciplineInspection{ins}
	r.attachInspectionViolations(ctx, schoolID, list)
	return list[0], nil
}

func (r *PostgresRepository) DeleteInspection(ctx context.Context, schoolID, id int64) error {
	affected, err := r.queries.DeleteDisciplineInspection(ctx, db.DeleteDisciplineInspectionParams{
		ID:       id,
		SchoolID: schoolID,
	})
	if err != nil {
		return mapPGError(err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
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

func mapCounselingRow(row db.GetCounselingRecordRow) CounselingRecord {
	return CounselingRecord{
		ID:            row.ID,
		SchoolID:      row.SchoolID,
		StudentID:     row.StudentID,
		CounselorID:   row.CounselorID,
		Tanggal:       Date(row.Tanggal),
		Topik:         row.Topik,
		Catatan:       row.Catatan,
		TindakLanjut:  row.TindakLanjut,
		Status:        row.Status,
		CreatedAt:     row.CreatedAt,
		NIS:           row.Nis,
		StudentName:   row.StudentName,
		NamaKelas:     row.NamaKelas,
		CounselorName: row.CounselorName,
	}
}

func (r *PostgresRepository) ListCounselingRecords(ctx context.Context, schoolID int64, filter ListCounselingFilter) ([]CounselingRecord, error) {
	rows, err := r.queries.ListCounselingRecords(ctx, db.ListCounselingRecordsParams{
		SchoolID:  schoolID,
		StudentID: filter.StudentID,
		Status:    filter.Status,
	})
	if err != nil {
		return nil, err
	}
	res := make([]CounselingRecord, 0, len(rows))
	for _, row := range rows {
		res = append(res, CounselingRecord{
			ID:            row.ID,
			SchoolID:      row.SchoolID,
			StudentID:     row.StudentID,
			CounselorID:   row.CounselorID,
			Tanggal:       Date(row.Tanggal),
			Topik:         row.Topik,
			Catatan:       row.Catatan,
			TindakLanjut:  row.TindakLanjut,
			Status:        row.Status,
			CreatedAt:     row.CreatedAt,
			NIS:           row.Nis,
			StudentName:   row.StudentName,
			NamaKelas:     row.NamaKelas,
			CounselorName: row.CounselorName,
		})
	}
	return res, nil
}

func (r *PostgresRepository) CreateCounselingRecord(ctx context.Context, schoolID int64, counselorID *int64, in CreateCounselingInput, tanggal time.Time) (CounselingRecord, error) {
	id, err := r.queries.CreateCounselingRecord(ctx, db.CreateCounselingRecordParams{
		SchoolID:     schoolID,
		StudentID:    in.StudentID,
		CounselorID:  counselorID,
		Tanggal:      tanggal,
		Topik:        in.Topik,
		Catatan:      in.Catatan,
		TindakLanjut: in.TindakLanjut,
		Status:       in.Status,
	})
	if err != nil {
		return CounselingRecord{}, mapPGError(err)
	}
	row, err := r.queries.GetCounselingRecord(ctx, db.GetCounselingRecordParams{ID: id, SchoolID: schoolID})
	if err != nil {
		return CounselingRecord{}, mapPGError(err)
	}
	return mapCounselingRow(row), nil
}

func (r *PostgresRepository) UpdateCounselingRecord(ctx context.Context, schoolID, id int64, in UpdateCounselingInput, tanggal time.Time) (CounselingRecord, error) {
	updatedID, err := r.queries.UpdateCounselingRecord(ctx, db.UpdateCounselingRecordParams{
		Tanggal:      tanggal,
		Topik:        in.Topik,
		Catatan:      in.Catatan,
		TindakLanjut: in.TindakLanjut,
		Status:       in.Status,
		ID:           id,
		SchoolID:     schoolID,
	})
	if err != nil {
		return CounselingRecord{}, mapPGError(err)
	}
	row, err := r.queries.GetCounselingRecord(ctx, db.GetCounselingRecordParams{ID: updatedID, SchoolID: schoolID})
	if err != nil {
		return CounselingRecord{}, mapPGError(err)
	}
	return mapCounselingRow(row), nil
}

func (r *PostgresRepository) DeleteCounselingRecord(ctx context.Context, schoolID, id int64) error {
	affected, err := r.queries.DeleteCounselingRecord(ctx, db.DeleteCounselingRecordParams{
		ID:       id,
		SchoolID: schoolID,
	})
	if err != nil {
		return mapPGError(err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func ptrTime(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

func mapParentSummonsRow(row db.GetParentSummonsRow) ParentSummons {
	return ParentSummons{
		ID:              row.ID,
		SchoolID:        row.SchoolID,
		StudentID:       row.StudentID,
		CreatedBy:       row.CreatedBy,
		NomorSurat:      row.NomorSurat,
		TanggalSurat:    Date(row.TanggalSurat),
		JadwalPertemuan: row.JadwalPertemuan,
		Ruangan:         row.Ruangan,
		Alasan:          row.Alasan,
		NamaOrtu:        row.NamaOrtu,
		NoHpOrtu:        row.NoHpOrtu,
		Status:          row.Status,
		SentAt:          ptrTime(row.SentAt),
		CompletedAt:     ptrTime(row.CompletedAt),
		CatatanHasil:    row.CatatanHasil,
		CreatedAt:       row.CreatedAt,
		NIS:             row.Nis,
		StudentName:     row.StudentName,
		NamaKelas:       row.NamaKelas,
		CreatedByName:   row.CreatedByName,
		TotalPoints:     row.TotalPoints,
	}
}

func (r *PostgresRepository) ListParentSummons(ctx context.Context, schoolID int64, filter ListSummonsFilter) ([]ParentSummons, error) {
	rows, err := r.queries.ListParentSummons(ctx, db.ListParentSummonsParams{
		SchoolID:  schoolID,
		StudentID: filter.StudentID,
		Status:    filter.Status,
	})
	if err != nil {
		return nil, err
	}
	res := make([]ParentSummons, 0, len(rows))
	for _, row := range rows {
		res = append(res, ParentSummons{
			ID:              row.ID,
			SchoolID:        row.SchoolID,
			StudentID:       row.StudentID,
			CreatedBy:       row.CreatedBy,
			NomorSurat:      row.NomorSurat,
			TanggalSurat:    Date(row.TanggalSurat),
			JadwalPertemuan: row.JadwalPertemuan,
			Ruangan:         row.Ruangan,
			Alasan:          row.Alasan,
			NamaOrtu:        row.NamaOrtu,
			NoHpOrtu:        row.NoHpOrtu,
			Status:          row.Status,
			SentAt:          ptrTime(row.SentAt),
			CompletedAt:     ptrTime(row.CompletedAt),
			CatatanHasil:    row.CatatanHasil,
			CreatedAt:       row.CreatedAt,
			NIS:             row.Nis,
			StudentName:     row.StudentName,
			NamaKelas:       row.NamaKelas,
			CreatedByName:   row.CreatedByName,
			TotalPoints:     row.TotalPoints,
		})
	}
	return res, nil
}

func (r *PostgresRepository) CreateParentSummons(ctx context.Context, schoolID int64, createdBy *int64, in CreateParentSummonsInput, tanggalSurat, jadwal time.Time) (ParentSummons, error) {
	id, err := r.queries.CreateParentSummons(ctx, db.CreateParentSummonsParams{
		SchoolID:        schoolID,
		StudentID:       in.StudentID,
		CreatedBy:       createdBy,
		NomorSurat:      in.NomorSurat,
		TanggalSurat:    tanggalSurat,
		JadwalPertemuan: jadwal,
		Ruangan:         in.Ruangan,
		Alasan:          in.Alasan,
		NamaOrtu:        in.NamaOrtu,
		NoHpOrtu:        in.NoHpOrtu,
		Status:          in.Status,
	})
	if err != nil {
		return ParentSummons{}, mapPGError(err)
	}
	row, err := r.queries.GetParentSummons(ctx, db.GetParentSummonsParams{ID: id, SchoolID: schoolID})
	if err != nil {
		return ParentSummons{}, mapPGError(err)
	}
	return mapParentSummonsRow(row), nil
}

func (r *PostgresRepository) UpdateParentSummons(ctx context.Context, schoolID, id int64, in UpdateParentSummonsInput, tanggalSurat, jadwal time.Time) (ParentSummons, error) {
	updatedID, err := r.queries.UpdateParentSummons(ctx, db.UpdateParentSummonsParams{
		NomorSurat:      in.NomorSurat,
		TanggalSurat:    tanggalSurat,
		JadwalPertemuan: jadwal,
		Ruangan:         in.Ruangan,
		Alasan:          in.Alasan,
		NamaOrtu:        in.NamaOrtu,
		NoHpOrtu:        in.NoHpOrtu,
		CatatanHasil:    in.CatatanHasil,
		ID:              id,
		SchoolID:        schoolID,
	})
	if err != nil {
		return ParentSummons{}, mapPGError(err)
	}
	row, err := r.queries.GetParentSummons(ctx, db.GetParentSummonsParams{ID: updatedID, SchoolID: schoolID})
	if err != nil {
		return ParentSummons{}, mapPGError(err)
	}
	return mapParentSummonsRow(row), nil
}

func (r *PostgresRepository) UpdateParentSummonsStatus(ctx context.Context, schoolID, id int64, status string, catatanHasil *string) (ParentSummons, error) {
	updatedID, err := r.queries.UpdateParentSummonsStatus(ctx, db.UpdateParentSummonsStatusParams{
		Status:       status,
		CatatanHasil: catatanHasil,
		ID:           id,
		SchoolID:     schoolID,
	})
	if err != nil {
		return ParentSummons{}, mapPGError(err)
	}
	row, err := r.queries.GetParentSummons(ctx, db.GetParentSummonsParams{ID: updatedID, SchoolID: schoolID})
	if err != nil {
		return ParentSummons{}, mapPGError(err)
	}
	return mapParentSummonsRow(row), nil
}

func (r *PostgresRepository) DeleteParentSummons(ctx context.Context, schoolID, id int64) error {
	affected, err := r.queries.DeleteParentSummons(ctx, db.DeleteParentSummonsParams{
		ID:       id,
		SchoolID: schoolID,
	})
	if err != nil {
		return mapPGError(err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
