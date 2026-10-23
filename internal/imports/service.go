package imports

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

var errImportAlreadyStarted = errors.New("import already started")

type Service struct {
	pool           *pgxpool.Pool
	queries        *db.Queries
	log            *slog.Logger
	dir            string
	maxFileSize    int64
	maxRows        int
	fileRetention  time.Duration
	errorRetention time.Duration
}

func (s *Service) MaxFileSize() int64 {
	return s.maxFileSize
}

func NewService(pool *pgxpool.Pool, log *slog.Logger, cfg Config) (*Service, error) {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(cfg.Dir) == "" {
		cfg.Dir = "./storage/imports"
	}
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = 5 << 20
	}
	if cfg.MaxRows <= 0 {
		cfg.MaxRows = 2000
	}
	if cfg.FileRetention <= 0 {
		cfg.FileRetention = 7 * 24 * time.Hour
	}
	if cfg.ErrorRetention <= 0 {
		cfg.ErrorRetention = 30 * 24 * time.Hour
	}
	dir, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("resolve import directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create import directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure import directory: %w", err)
	}
	return &Service{
		pool:           pool,
		queries:        db.New(pool),
		log:            log,
		dir:            dir,
		maxFileSize:    cfg.MaxFileSize,
		maxRows:        cfg.MaxRows,
		fileRetention:  cfg.FileRetention,
		errorRetention: cfg.ErrorRetention,
	}, nil
}

func (s *Service) Upload(ctx context.Context, actorID string, entity Entity, mode Mode, filename string, data []byte, requestID string) (UploadResult, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return UploadResult{}, ErrInvalidInput
	}
	if entity != EntityStudents && entity != EntityTeachers {
		return UploadResult{}, ErrInvalidInput
	}
	if mode != ModeCreateOnly && mode != ModeUpsert {
		return UploadResult{}, ErrInvalidInput
	}
	if len(data) == 0 {
		return UploadResult{}, ErrInvalidInput
	}
	if int64(len(data)) > s.maxFileSize {
		return UploadResult{}, ErrFileTooLarge
	}
	filename, err = cleanText(filepath.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/")), 255)
	if err != nil || filename == "." || !strings.HasSuffix(strings.ToLower(filename), ".xlsx") {
		return UploadResult{}, ErrInvalidInput
	}

	validation, err := s.validateFile(ctx, s.queries, entity, mode, data)
	if err != nil {
		return UploadResult{}, fmt.Errorf("validate import file: %w", err)
	}
	status := "validated"
	if validation.TotalRows == 0 || validation.InvalidRows > 0 || validation.DuplicateRows > 0 {
		status = "invalid"
	}
	if len(validation.Errors) == 0 && status == "invalid" {
		validation.Errors = append(validation.Errors, RowError{RowNumber: 0, Column: "file", Code: "empty_file", Message: "file tidak memiliki baris data"})
	}

	jobID := uuid.New()
	storageKey := jobID.String() + ".xlsx"
	if err := s.writeFile(storageKey, data); err != nil {
		return UploadResult{}, fmt.Errorf("store import file: %w", err)
	}

	var job db.ImportJob
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		created, err := q.CreateImportJob(ctx, db.CreateImportJobParams{
			EntityType:       string(entity),
			ImportMode:       string(mode),
			OriginalFilename: filename,
			FileSha256:       hashData(data),
			FileStorageKey:   optionalText(storageKey),
			Status:           status,
			TotalRows:        int32(validation.TotalRows),
			ValidRows:        int32(validation.ValidRows),
			DuplicateRows:    int32(validation.DuplicateRows),
			InvalidRows:      int32(validation.InvalidRows),
			StartedByUserID:  actorUUID,
		})
		if err != nil {
			return err
		}
		job = created
		for _, rowErr := range validation.Errors {
			if err := q.CreateImportRowError(ctx, db.CreateImportRowErrorParams{
				ImportJobID: created.ID,
				RowNumber:   int32(rowErr.RowNumber),
				ColumnName:  optionalText(rowErr.Column),
				ErrorCode:   rowErr.Code,
				Message:     rowErr.Message,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = s.removeFile(storageKey)
		return UploadResult{}, fmt.Errorf("create import job: %w", err)
	}
	return UploadResult{
		Job:     jobFromDB(job),
		Preview: nonNilPreview(validation.Rows),
		Errors:  nonNilErrors(validation.Errors),
	}, nil
}

func (s *Service) Confirm(ctx context.Context, actorID, jobID, requestID string) (Job, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return Job{}, ErrInvalidInput
	}
	id, err := parseUUID(jobID)
	if err != nil {
		return Job{}, ErrInvalidInput
	}
	job, err := s.queries.GetImportJob(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Job{}, ErrNotFound
		}
		return Job{}, fmt.Errorf("get import job: %w", err)
	}
	if job.Status != "validated" {
		return Job{}, ErrConflict
	}
	data, err := s.readFile(job.FileStorageKey.String)
	if err != nil {
		_ = s.failJob(ctx, id, actorUUID, "failed", "file_missing", "file import tidak ditemukan")
		return Job{}, ErrConflict
	}
	if hashData(data) != job.FileSha256 {
		_ = s.failJob(ctx, id, actorUUID, "failed", "checksum_mismatch", "checksum file import tidak cocok")
		return Job{}, ErrConflict
	}

	var result db.ImportJob
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		started, err := q.StartImportJob(ctx, db.StartImportJobParams{
			ConfirmedByUserID: actorUUID,
			ID:                id,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return errImportAlreadyStarted
			}
			return fmt.Errorf("start import job: %w", err)
		}
		validation, err := s.validateFile(ctx, q, Entity(started.EntityType), Mode(started.ImportMode), data)
		if err != nil {
			return err
		}
		if validation.TotalRows != int(started.TotalRows) ||
			validation.ValidRows != int(started.ValidRows) ||
			validation.DuplicateRows != int(started.DuplicateRows) ||
			validation.InvalidRows != int(started.InvalidRows) {
			return ErrPreviewChanged
		}
		if validation.TotalRows == 0 || validation.InvalidRows > 0 || validation.DuplicateRows > 0 {
			return ErrPreviewChanged
		}
		switch Entity(started.EntityType) {
		case EntityStudents:
			if err := s.applyStudents(ctx, q, validation.studentRows, actorUUID); err != nil {
				return err
			}
		case EntityTeachers:
			if err := s.applyTeachers(ctx, q, validation.teacherRows); err != nil {
				return err
			}
		default:
			return ErrInvalidInput
		}
		if err := q.CompleteImportJob(ctx, db.CompleteImportJobParams{
			SuccessRows: int32(validation.ValidRows),
			ID:          id,
		}); err != nil {
			return fmt.Errorf("complete import job: %w", err)
		}
		if err := s.auditData(ctx, q, actorUUID, "import.complete", id, map[string]any{
			"entity_type":    started.EntityType,
			"import_mode":    started.ImportMode,
			"total_rows":     validation.TotalRows,
			"success_rows":   validation.ValidRows,
			"duplicate_rows": validation.DuplicateRows,
			"invalid_rows":   validation.InvalidRows,
		}, requestID); err != nil {
			return fmt.Errorf("write import audit: %w", err)
		}
		completed, err := q.GetImportJob(ctx, id)
		if err != nil {
			return fmt.Errorf("get completed import job: %w", err)
		}
		result = completed
		return nil
	})
	if err != nil {
		if errors.Is(err, errImportAlreadyStarted) {
			return Job{}, ErrConflict
		}
		if errors.Is(err, ErrPreviewChanged) {
			_ = s.failJob(ctx, id, actorUUID, "invalid", "preview_changed", "data berubah sejak preview; unggah ulang file")
			return Job{}, ErrPreviewChanged
		}
		_ = s.failJob(ctx, id, actorUUID, "failed", "apply_failed", "import gagal diterapkan")
		return Job{}, mapWriteError(err)
	}
	return jobFromDB(result), nil
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Job, int64, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.EntityType != "" && filter.EntityType != string(EntityStudents) && filter.EntityType != string(EntityTeachers) {
		return nil, 0, ErrInvalidInput
	}
	if filter.Status != "" && !validJobStatus(filter.Status) {
		return nil, 0, ErrInvalidInput
	}
	params := db.ListImportJobsParams{
		EntityType: optionalText(filter.EntityType),
		Status:     optionalText(filter.Status),
		PageLimit:  filter.Limit,
		PageOffset: filter.Offset,
	}
	rows, err := s.queries.ListImportJobs(ctx, params)
	if err != nil {
		return nil, 0, fmt.Errorf("list import jobs: %w", err)
	}
	total, err := s.queries.CountImportJobs(ctx, db.CountImportJobsParams{
		EntityType: params.EntityType,
		Status:     params.Status,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("count import jobs: %w", err)
	}
	jobs := make([]Job, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, jobFromDB(row))
	}
	return jobs, total, nil
}

func (s *Service) Get(ctx context.Context, jobID string) (Detail, error) {
	id, err := parseUUID(jobID)
	if err != nil {
		return Detail{}, ErrInvalidInput
	}
	job, err := s.queries.GetImportJob(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, fmt.Errorf("get import job: %w", err)
	}
	errorRows, err := s.queries.ListImportRowErrors(ctx, id)
	if err != nil {
		return Detail{}, fmt.Errorf("list import errors: %w", err)
	}
	detail := Detail{
		Job:    jobFromDB(job),
		Errors: make([]RowError, 0, len(errorRows)),
	}
	for _, row := range errorRows {
		detail.Errors = append(detail.Errors, rowErrorFromDB(row))
	}
	if job.Status == "validated" && job.FileStorageKey.Valid {
		data, err := s.readFile(job.FileStorageKey.String)
		if err == nil {
			validation, validateErr := s.validateFile(ctx, s.queries, Entity(job.EntityType), Mode(job.ImportMode), data)
			if validateErr == nil {
				detail.Preview = nonNilPreview(validation.Rows)
			}
		}
	}
	return detail, nil
}

func (s *Service) validateFile(ctx context.Context, q *db.Queries, entity Entity, mode Mode, data []byte) (Validation, error) {
	if err := checkArchiveSize(data, 64<<20); err != nil {
		code := "invalid_file"
		message := "file bukan berkas Excel yang valid"
		if errors.Is(err, ErrFileTooLarge) {
			code = "file_too_large"
			message = "isi file Excel terlalu besar setelah diekstrak"
		}
		return Validation{TotalRows: 0, InvalidRows: 1, Errors: []RowError{{RowNumber: 0, Column: "file", Code: code, Message: message}}}, nil
	}
	switch entity {
	case EntityStudents:
		rows, rowErr := parseStudentWorkbook(data, s.maxRows)
		if rowErr != nil {
			return Validation{TotalRows: 0, InvalidRows: 1, Errors: []RowError{*rowErr}}, nil
		}
		return s.validateStudents(ctx, q, mode, rows)
	case EntityTeachers:
		rows, rowErr := parseTeacherWorkbook(data, s.maxRows)
		if rowErr != nil {
			return Validation{TotalRows: 0, InvalidRows: 1, Errors: []RowError{*rowErr}}, nil
		}
		return s.validateTeachers(ctx, q, mode, rows)
	default:
		return Validation{TotalRows: 0, InvalidRows: 1, Errors: []RowError{{RowNumber: 0, Column: "file", Code: "unknown_entity", Message: "jenis data tidak dikenal"}}}, nil
	}
}

func (s *Service) failJob(ctx context.Context, id, actorID pgtype.UUID, status, code, message string) error {
	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		affected, err := q.FailImportJob(ctx, db.FailImportJobParams{Status: status, ID: id})
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
		if err := q.CreateImportRowError(ctx, db.CreateImportRowErrorParams{
			ImportJobID: id,
			RowNumber:   0,
			ColumnName:  optionalText("file"),
			ErrorCode:   code,
			Message:     message,
		}); err != nil {
			return err
		}
		return s.auditData(ctx, q, actorID, "import.failed", id, map[string]any{
			"status": status,
			"code":   code,
		}, "")
	})
}

func validJobStatus(value string) bool {
	switch value {
	case "validating", "validated", "invalid", "processing", "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func nonNilPreview(rows []PreviewRow) []PreviewRow {
	if rows == nil {
		return []PreviewRow{}
	}
	return rows
}

func nonNilErrors(rows []RowError) []RowError {
	if rows == nil {
		return []RowError{}
	}
	return rows
}

func (s *Service) writeFile(key string, data []byte) error {
	key = filepath.Base(key)
	if key == "." || key == ".." || key == "" {
		return ErrInvalidInput
	}
	return os.WriteFile(filepath.Join(s.dir, key), data, 0o600)
}

func (s *Service) readFile(key string) ([]byte, error) {
	key = filepath.Base(key)
	if key == "." || key == ".." || key == "" {
		return nil, ErrNotFound
	}
	return os.ReadFile(filepath.Join(s.dir, key))
}

func (s *Service) removeFile(key string) error {
	key = filepath.Base(key)
	if key == "." || key == ".." || key == "" {
		return nil
	}
	err := os.Remove(filepath.Join(s.dir, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// RunCleanup removes expired import files and row errors. It runs once on
// startup and then daily until ctx is cancelled.
func (s *Service) RunCleanup(ctx context.Context) {
	if err := s.Cleanup(ctx); err != nil {
		s.log.Warn("import_cleanup_failed", "error", err)
	}
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Cleanup(ctx); err != nil {
				s.log.Warn("import_cleanup_failed", "error", err)
			}
		}
	}
}

func (s *Service) Cleanup(ctx context.Context) error {
	fileCutoff := time.Now().Add(-s.fileRetention)
	rows, err := s.queries.ListImportJobsForFileCleanup(ctx, pgtype.Timestamptz{Time: fileCutoff, Valid: true})
	if err != nil {
		return fmt.Errorf("list expired import files: %w", err)
	}
	for _, row := range rows {
		if !row.FileStorageKey.Valid {
			continue
		}
		if err := s.removeFile(row.FileStorageKey.String); err != nil {
			s.log.Warn("import_file_cleanup_failed", "import_job_id", uuidString(row.ID), "error", err)
			continue
		}
		if err := s.queries.ClearImportJobFile(ctx, row.ID); err != nil {
			return fmt.Errorf("clear import file key: %w", err)
		}
	}
	errorCutoff := time.Now().Add(-s.errorRetention)
	if err := s.queries.DeleteOldImportRowErrors(ctx, pgtype.Timestamptz{Time: errorCutoff, Valid: true}); err != nil {
		return fmt.Errorf("delete expired import errors: %w", err)
	}
	return nil
}
