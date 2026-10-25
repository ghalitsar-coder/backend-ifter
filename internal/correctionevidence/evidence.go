package correctionevidence

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

const auditEntityType = "attendance_correction_evidence"

// Upload stores one evidence document for an attendance record's correction
// flow, links it to a correction (explicit, or the newest one on the record),
// writes the correction_evidence.add audit row and returns the item.
func (s *Service) Upload(ctx context.Context, actor Actor, recordIDValue string, in UploadInput, requestID string) (EvidenceItem, error) {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return EvidenceItem{}, ErrUnauthorized
	}
	recordID, err := parseUUID(recordIDValue)
	if err != nil {
		return EvidenceItem{}, ErrInvalidInput
	}
	if _, err := s.checkRecordAccess(ctx, actor, recordID); err != nil {
		return EvidenceItem{}, err
	}
	if len(in.Data) == 0 {
		return EvidenceItem{}, ErrInvalidInput
	}
	if int64(len(in.Data)) > s.maxFileSize {
		return EvidenceItem{}, ErrFileTooLarge
	}
	extension, err := validateDocument(in.Filename, in.ContentType, in.Data)
	if err != nil {
		return EvidenceItem{}, err
	}
	filename, err := cleanText(filepath.Base(in.Filename), 255)
	if err != nil || filename == "." || filename == "" {
		return EvidenceItem{}, ErrInvalidInput
	}
	note, err := cleanText(in.Note, 1000)
	if err != nil {
		return EvidenceItem{}, ErrInvalidInput
	}
	correctionID, err := s.resolveCorrectionID(ctx, recordID, in.CorrectionID)
	if err != nil {
		return EvidenceItem{}, err
	}
	if err := s.ensureDir(); err != nil {
		return EvidenceItem{}, err
	}
	storedKey := uuid.NewString() + extension
	if err := s.writeFile(storedKey, in.Data); err != nil {
		return EvidenceItem{}, fmt.Errorf("store correction evidence file: %w", err)
	}

	var row db.AttendanceCorrectionEvidence
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		created, err := q.CreateCorrectionEvidence(ctx, db.CreateCorrectionEvidenceParams{
			AttendanceRecordID: recordID,
			CorrectionID:       correctionID,
			FileName:           filename,
			ContentType:        allowedDocumentExtensions[extension],
			SizeBytes:          int64(len(in.Data)),
			StoredKey:          storedKey,
			UploadedBy:         actorID,
			Note:               optionalText(note),
		})
		if err != nil {
			return mapWriteError(fmt.Errorf("create correction evidence: %w", err))
		}
		row = created
		after := itemFromModelRow(created)
		if err := s.auditData(ctx, q, actorID, "correction_evidence.add", auditEntityType,
			created.ID, nil, after, requestID); err != nil {
			return mapWriteError(fmt.Errorf("audit correction evidence add: %w", err))
		}
		return nil
	})
	if err != nil {
		if cleanupErr := s.removeFile(storedKey); cleanupErr != nil {
			s.log.Error("cleanup_correction_evidence_file", "stored_key", storedKey, "error", cleanupErr)
		}
		return EvidenceItem{}, err
	}
	return itemFromModelRow(row), nil
}

// List returns the evidence attachments of one attendance record (no binary,
// never the stored key), scoped like Upload.
func (s *Service) List(ctx context.Context, actor Actor, recordIDValue string) ([]EvidenceItem, error) {
	recordID, err := parseUUID(recordIDValue)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if _, err := s.checkRecordAccess(ctx, actor, recordID); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListCorrectionEvidence(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("list correction evidence: %w", err)
	}
	items := make([]EvidenceItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, EvidenceItem{
			ID:                 uuidString(row.ID),
			AttendanceRecordID: uuidString(row.AttendanceRecordID),
			CorrectionID:       nullableUUIDString(row.CorrectionID),
			FileName:           row.FileName,
			ContentType:        row.ContentType,
			SizeBytes:          row.SizeBytes,
			Note:               textPtr(row.Note),
			UploadedBy:         uuidString(row.UploadedBy),
			CreatedAt:          timestampValue(row.CreatedAt),
		})
	}
	return items, nil
}

// Download resolves one evidence document; the private file is served from
// the imports storage directory and never enumerable via a static route.
func (s *Service) Download(ctx context.Context, actor Actor, evidenceIDValue string) (DocumentDownload, error) {
	evidenceID, err := parseUUID(evidenceIDValue)
	if err != nil {
		return DocumentDownload{}, ErrInvalidInput
	}
	row, err := s.queries.GetCorrectionEvidence(ctx, evidenceID)
	if err != nil {
		if database.IsNotFound(err) {
			return DocumentDownload{}, ErrNotFound
		}
		return DocumentDownload{}, fmt.Errorf("load correction evidence: %w", err)
	}
	if _, err := s.checkRecordAccess(ctx, actor, row.AttendanceRecordID); err != nil {
		return DocumentDownload{}, err
	}
	data, err := s.readFile(row.StoredKey)
	if err != nil {
		return DocumentDownload{}, fmt.Errorf("read correction evidence file: %w", err)
	}
	return DocumentDownload{Filename: row.FileName, ContentType: row.ContentType, Data: data}, nil
}

// Delete removes one evidence row (and its file after the commit) with a
// correction_evidence.delete audit entry.
func (s *Service) Delete(ctx context.Context, actor Actor, evidenceIDValue string, requestID string) error {
	evidenceID, err := parseUUID(evidenceIDValue)
	if err != nil {
		return ErrInvalidInput
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return ErrUnauthorized
	}
	row, err := s.queries.GetCorrectionEvidence(ctx, evidenceID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("load correction evidence: %w", err)
	}
	if _, err := s.checkRecordAccess(ctx, actor, row.AttendanceRecordID); err != nil {
		return err
	}
	before := itemFromModelRow(row)
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := q.DeleteCorrectionEvidence(ctx, evidenceID); err != nil {
			return mapWriteError(fmt.Errorf("delete correction evidence: %w", err))
		}
		if err := s.auditData(ctx, q, actorID, "correction_evidence.delete", auditEntityType,
			evidenceID, before, nil, requestID); err != nil {
			return mapWriteError(fmt.Errorf("audit correction evidence delete: %w", err))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.removeFile(row.StoredKey); err != nil {
		s.log.Error("remove_correction_evidence_file", "stored_key", row.StoredKey, "error", err)
	}
	return nil
}

// resolveCorrectionID picks the correction an upload attaches to: the
// explicit (validated) id, otherwise the newest correction on the record,
// otherwise none (correction_id stays null).
func (s *Service) resolveCorrectionID(ctx context.Context, recordID pgtype.UUID, explicit string) (pgtype.UUID, error) {
	if explicit != "" {
		id, err := parseUUID(explicit)
		if err != nil {
			return pgtype.UUID{}, ErrInvalidInput
		}
		linked, err := s.queries.GetAttendanceCorrectionForRecord(ctx, db.GetAttendanceCorrectionForRecordParams{
			ID:                 id,
			AttendanceRecordID: recordID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return pgtype.UUID{}, ErrInvalidInput
			}
			return pgtype.UUID{}, fmt.Errorf("resolve correction for evidence: %w", err)
		}
		return linked, nil
	}
	latest, err := s.queries.LatestAttendanceCorrectionForRecord(ctx, recordID)
	if err != nil {
		if database.IsNotFound(err) {
			return pgtype.UUID{}, nil
		}
		return pgtype.UUID{}, fmt.Errorf("latest correction for evidence: %w", err)
	}
	return latest, nil
}

// checkRecordAccess loads the record and applies the class scope; it returns
// the record row so callers can reuse the lookup.
func (s *Service) checkRecordAccess(ctx context.Context, actor Actor, recordID pgtype.UUID) (db.GetAttendanceRecordForEvidenceRow, error) {
	record, err := s.queries.GetAttendanceRecordForEvidence(ctx, recordID)
	if err != nil {
		if database.IsNotFound(err) {
			return record, ErrNotFound
		}
		return record, fmt.Errorf("load attendance record for evidence: %w", err)
	}
	if err := s.checkClassAccess(ctx, actor, record.ClassID, record.AttendanceDate); err != nil {
		return record, err
	}
	return record, nil
}

// checkClassAccess mirrors the attendance correction scope
// (internal/attendance canWriteClass): admin school-wide; teacher/duty_teacher
// through their active teacher profile, homeroom on the record date or duty on
// that day. The AttendanceOps role gate already excludes principal/staff_tu,
// this is the data scoping on top of it.
func (s *Service) checkClassAccess(ctx context.Context, actor Actor, classID pgtype.UUID, date pgtype.Date) error {
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return ErrUnauthorized
	}
	if hasRole(actor.Roles, "admin") {
		return nil
	}
	if !hasRole(actor.Roles, "teacher") && !hasRole(actor.Roles, "duty_teacher") {
		return ErrForbidden
	}
	teacher, err := s.queries.GetTeacherByUserID(ctx, actorID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrForbidden
		}
		return fmt.Errorf("find teacher for evidence actor: %w", err)
	}
	if teacher.Status != "active" {
		return ErrForbidden
	}
	if hasRole(actor.Roles, "teacher") {
		homeroom, err := s.queries.HasHomeroomAssignmentForDate(ctx, db.HasHomeroomAssignmentForDateParams{
			ClassID:        classID,
			TeacherID:      teacher.ID,
			AttendanceDate: date,
		})
		if err != nil {
			return fmt.Errorf("check homeroom scope for evidence: %w", err)
		}
		if homeroom {
			return nil
		}
	}
	if hasRole(actor.Roles, "duty_teacher") {
		onDuty, err := s.queries.HasDutyAssignmentForDate(ctx, db.HasDutyAssignmentForDateParams{
			TeacherID:      teacher.ID,
			AttendanceDate: date,
		})
		if err != nil {
			return fmt.Errorf("check duty scope for evidence: %w", err)
		}
		if onDuty {
			return nil
		}
	}
	return ErrForbidden
}

func itemFromModelRow(row db.AttendanceCorrectionEvidence) EvidenceItem {
	return EvidenceItem{
		ID:                 uuidString(row.ID),
		AttendanceRecordID: uuidString(row.AttendanceRecordID),
		CorrectionID:       nullableUUIDString(row.CorrectionID),
		FileName:           row.FileName,
		ContentType:        row.ContentType,
		SizeBytes:          row.SizeBytes,
		Note:               textPtr(row.Note),
		UploadedBy:         uuidString(row.UploadedBy),
		CreatedAt:          timestampValue(row.CreatedAt),
	}
}
