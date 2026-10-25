package masterdata

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// maxStudentNoteLength is the API limit for one administrative note; the
// migration only enforces a non-blank body, so the length cap lives here.
const maxStudentNoteLength = 2000

// ListStudentNotes returns every note on a student, newest first. An unknown
// student is a 404 rather than an empty list.
func (s *Service) ListStudentNotes(ctx context.Context, studentID string) ([]StudentNote, error) {
	id, err := parseUUID(studentID)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if _, err := s.queries.GetStudent(ctx, id); err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get student for notes: %w", err)
	}
	rows, err := s.queries.ListStudentNotes(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list student notes: %w", err)
	}
	notes := make([]StudentNote, 0, len(rows))
	for _, row := range rows {
		notes = append(notes, studentNoteFromList(row))
	}
	return notes, nil
}

// CreateStudentNote appends one note to a student and audits it as
// student_note.create. The student row is locked first so a concurrent delete
// cannot slip between the existence check and the insert.
func (s *Service) CreateStudentNote(ctx context.Context, actorID, studentID, note, requestID string) (StudentNote, error) {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return StudentNote{}, ErrUnauthorized
	}
	studentUUID, err := parseUUID(studentID)
	if err != nil {
		return StudentNote{}, ErrInvalidInput
	}
	content, err := requiredText(note, maxStudentNoteLength)
	if err != nil {
		return StudentNote{}, err
	}

	var result StudentNote
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.GetStudentForUpdate(ctx, studentUUID); err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get student for note: %w", err)
		}
		row, err := q.CreateStudentNote(ctx, db.CreateStudentNoteParams{
			StudentID: studentUUID,
			Note:      content,
			CreatedBy: actorUUID,
		})
		if err != nil {
			return mapWriteError(err)
		}
		result = studentNoteFromCreate(row)
		if err := s.auditData(ctx, q, actorUUID, "student_note.create", "student_note", row.ID, nil, toStudentNoteAudit(result), requestID); err != nil {
			return fmt.Errorf("write student note audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return StudentNote{}, mapWriteError(err)
	}
	return result, nil
}

// DeleteStudentNote removes one note scoped to its student and audits it as
// student_note.delete with the deleted body in before_data.
func (s *Service) DeleteStudentNote(ctx context.Context, actorID, studentID, noteID, requestID string) error {
	actorUUID, err := parseUUID(actorID)
	if err != nil {
		return ErrUnauthorized
	}
	studentUUID, err := parseUUID(studentID)
	if err != nil {
		return ErrInvalidInput
	}
	noteUUID, err := parseUUID(noteID)
	if err != nil {
		return ErrInvalidInput
	}

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.GetStudentForUpdate(ctx, studentUUID); err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get student for note delete: %w", err)
		}
		existing, err := q.GetStudentNote(ctx, db.GetStudentNoteParams{
			ID:        noteUUID,
			StudentID: studentUUID,
		})
		if err != nil {
			if database.IsNotFound(err) {
				return ErrNotFound
			}
			return fmt.Errorf("get student note: %w", err)
		}
		deleted, err := q.DeleteStudentNote(ctx, db.DeleteStudentNoteParams{
			ID:        noteUUID,
			StudentID: studentUUID,
		})
		if err != nil {
			return mapWriteError(err)
		}
		if deleted == 0 {
			return ErrNotFound
		}
		if err := s.auditData(ctx, q, actorUUID, "student_note.delete", "student_note", noteUUID, toStudentNoteAudit(studentNoteFromGet(existing)), nil, requestID); err != nil {
			return fmt.Errorf("write student note delete audit: %w", err)
		}
		return nil
	})
	return mapWriteError(err)
}

type studentNoteAudit struct {
	ID        string `json:"id"`
	StudentID string `json:"student_id"`
	Note      string `json:"note"`
	CreatedBy string `json:"created_by,omitempty"`
}

func toStudentNoteAudit(note StudentNote) studentNoteAudit {
	return studentNoteAudit{
		ID:        note.ID,
		StudentID: note.StudentID,
		Note:      note.Note,
		CreatedBy: note.CreatedBy,
	}
}

func studentNoteFromRow(id, studentID pgtype.UUID, note string, createdBy pgtype.UUID, createdByName string, createdAt pgtype.Timestamptz) StudentNote {
	return StudentNote{
		ID:            uuidString(id),
		StudentID:     uuidString(studentID),
		Note:          note,
		CreatedBy:     uuidString(createdBy),
		CreatedByName: createdByName,
		CreatedAt:     timestamp(createdAt),
	}
}

func studentNoteFromList(row db.ListStudentNotesRow) StudentNote {
	return studentNoteFromRow(row.ID, row.StudentID, row.Note, row.CreatedBy, row.CreatedByName, row.CreatedAt)
}

func studentNoteFromGet(row db.GetStudentNoteRow) StudentNote {
	return studentNoteFromRow(row.ID, row.StudentID, row.Note, row.CreatedBy, row.CreatedByName, row.CreatedAt)
}

func studentNoteFromCreate(row db.CreateStudentNoteRow) StudentNote {
	return studentNoteFromRow(row.ID, row.StudentID, row.Note, row.CreatedBy, row.CreatedByName, row.CreatedAt)
}
