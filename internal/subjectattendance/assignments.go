package subjectattendance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// ListTeachingAssignments is GET /teaching-assignments. A plain teacher only
// sees their own assignments; every other staff role sees the school.
func (s *Service) ListTeachingAssignments(ctx context.Context, actor Actor, filter TeachingAssignmentFilter) (TeachingAssignmentList, error) {
	if err := requireReadActor(actor); err != nil {
		return TeachingAssignmentList{}, err
	}
	params := db.ListTeachingAssignmentsParams{IsActive: boolParam(filter.IsActive)}
	for name, raw := range map[string]string{
		"teacher_id": filter.TeacherID,
		"subject_id": filter.SubjectID,
		"class_id":   filter.ClassID,
	} {
		if raw == "" {
			continue
		}
		id, err := parseUUID(raw)
		if err != nil {
			return TeachingAssignmentList{}, ErrInvalidInput
		}
		switch name {
		case "teacher_id":
			params.TeacherID = id
		case "subject_id":
			params.SubjectID = id
		case "class_id":
			params.ClassID = id
		}
	}
	if teacherOnly(actor) {
		scope, err := s.resolveScope(ctx, actor, false)
		if err != nil {
			return TeachingAssignmentList{}, err
		}
		params.TeacherID = scope.teacherID
	}
	rows, err := s.queries.ListTeachingAssignments(ctx, params)
	if err != nil {
		return TeachingAssignmentList{}, fmt.Errorf("list teaching assignments: %w", err)
	}
	items := make([]TeachingAssignment, 0, len(rows))
	for _, row := range rows {
		items = append(items, assignmentOfList(row))
	}
	return TeachingAssignmentList{Data: items}, nil
}

// GetTeachingAssignment is GET /teaching-assignments/:id.
func (s *Service) GetTeachingAssignment(ctx context.Context, actor Actor, idValue string) (TeachingAssignment, error) {
	if err := requireReadActor(actor); err != nil {
		return TeachingAssignment{}, err
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return TeachingAssignment{}, ErrInvalidInput
	}
	row, err := s.queries.GetTeachingAssignment(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return TeachingAssignment{}, ErrNotFound
		}
		return TeachingAssignment{}, fmt.Errorf("get teaching assignment: %w", err)
	}
	if err := s.ensureAssignmentAccess(ctx, actor, row.TeacherID); err != nil {
		return TeachingAssignment{}, err
	}
	return assignmentOf(row), nil
}

// CreateTeachingAssignment is POST /teaching-assignments (admin + staff_tu),
// audited as teaching_assignment.create.
func (s *Service) CreateTeachingAssignment(ctx context.Context, actor Actor, input TeachingAssignmentInput, requestID string) (TeachingAssignment, error) {
	if err := requireMasterActor(actor); err != nil {
		return TeachingAssignment{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return TeachingAssignment{}, ErrUnauthorized
	}
	teacherID, subjectID, classID, err := s.validateAssignmentRefs(ctx, input)
	if err != nil {
		return TeachingAssignment{}, err
	}
	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var created db.GetTeachingAssignmentRow
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		row, err := q.CreateTeachingAssignment(ctx, db.CreateTeachingAssignmentParams{
			TeacherID: teacherID,
			SubjectID: subjectID,
			ClassID:   classID,
			IsActive:  isActive,
			CreatedBy: actorID,
		})
		if err != nil {
			return err
		}
		created, err = q.GetTeachingAssignment(ctx, row.ID)
		if err != nil {
			return err
		}
		return s.audit(ctx, q, actorID, "teaching_assignment.create", "teaching_assignment",
			row.ID, nil, assignmentOf(created), requestID)
	})
	if err != nil {
		return TeachingAssignment{}, mapWriteError(err)
	}
	return assignmentOf(created), nil
}

// UpdateTeachingAssignment is PUT /teaching-assignments/:id, audited with
// before/after. Existing sessions keep pointing at the assignment.
func (s *Service) UpdateTeachingAssignment(ctx context.Context, actor Actor, idValue string, input TeachingAssignmentInput, requestID string) (TeachingAssignment, error) {
	if err := requireMasterActor(actor); err != nil {
		return TeachingAssignment{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return TeachingAssignment{}, ErrUnauthorized
	}
	id, err := parseUUID(idValue)
	if err != nil {
		return TeachingAssignment{}, ErrInvalidInput
	}
	teacherID, subjectID, classID, err := s.validateAssignmentRefs(ctx, input)
	if err != nil {
		return TeachingAssignment{}, err
	}
	before, err := s.queries.GetTeachingAssignment(ctx, id)
	if err != nil {
		if database.IsNotFound(err) {
			return TeachingAssignment{}, ErrNotFound
		}
		return TeachingAssignment{}, fmt.Errorf("get teaching assignment: %w", err)
	}
	isActive := before.IsActive
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	var updated db.GetTeachingAssignmentRow
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if _, err := q.UpdateTeachingAssignment(ctx, db.UpdateTeachingAssignmentParams{
			TeacherID: teacherID,
			SubjectID: subjectID,
			ClassID:   classID,
			IsActive:  isActive,
			ID:        id,
		}); err != nil {
			return err
		}
		row, err := q.GetTeachingAssignment(ctx, id)
		if err != nil {
			return err
		}
		updated = row
		return s.audit(ctx, q, actorID, "teaching_assignment.update", "teaching_assignment",
			id, assignmentOf(before), assignmentOf(row), requestID)
	})
	if err != nil {
		return TeachingAssignment{}, mapWriteError(err)
	}
	return assignmentOf(updated), nil
}

// validateAssignmentRefs parses the triple and resolves every referenced row,
// so an unknown teacher/subject/class is 404 instead of a foreign-key 400.
func (s *Service) validateAssignmentRefs(ctx context.Context, input TeachingAssignmentInput) (pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	teacherID, err := parseUUID(input.TeacherID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrInvalidInput
	}
	subjectID, err := parseUUID(input.SubjectID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrInvalidInput
	}
	classID, err := parseUUID(input.ClassID)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrInvalidInput
	}
	if _, err := s.queries.GetTeacher(ctx, teacherID); err != nil {
		if database.IsNotFound(err) {
			return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrNotFound
		}
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("get assigned teacher: %w", err)
	}
	if _, err := s.queries.GetSubject(ctx, subjectID); err != nil {
		if database.IsNotFound(err) {
			return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrNotFound
		}
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("get assignment subject: %w", err)
	}
	if _, err := s.queries.GetClass(ctx, classID); err != nil {
		if database.IsNotFound(err) {
			return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, ErrNotFound
		}
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("get assignment class: %w", err)
	}
	return teacherID, subjectID, classID, nil
}

// ensureAssignmentAccess rejects a plain teacher reading another teacher's
// assignment. School-wide roles resolveScope returns restrict=false.
func (s *Service) ensureAssignmentAccess(ctx context.Context, actor Actor, teacherID pgtype.UUID) error {
	if !teacherOnly(actor) {
		return nil
	}
	scope, err := s.resolveScope(ctx, actor, false)
	if err != nil {
		return err
	}
	if !scope.allows(teacherID) {
		return ErrForbidden
	}
	return nil
}

func teacherOnly(actor Actor) bool {
	return hasRole(actor.Roles, RoleTeacher) &&
		!hasAnyRole(actor.Roles, RoleAdmin, RoleStaffTU, RoleDutyTeacher, RolePrincipal)
}

func assignmentOf(row db.GetTeachingAssignmentRow) TeachingAssignment {
	return TeachingAssignment{
		ID:          uuidString(row.ID),
		TeacherID:   uuidString(row.TeacherID),
		TeacherName: row.TeacherName,
		SubjectID:   uuidString(row.SubjectID),
		SubjectCode: row.SubjectCode,
		SubjectName: row.SubjectName,
		ClassID:     uuidString(row.ClassID),
		ClassName:   row.ClassName,
		IsActive:    row.IsActive,
		CreatedAt:   timestamp(row.CreatedAt),
		UpdatedAt:   timestamp(row.UpdatedAt),
	}
}

func assignmentOfList(row db.ListTeachingAssignmentsRow) TeachingAssignment {
	return TeachingAssignment{
		ID:          uuidString(row.ID),
		TeacherID:   uuidString(row.TeacherID),
		TeacherName: row.TeacherName,
		SubjectID:   uuidString(row.SubjectID),
		SubjectCode: row.SubjectCode,
		SubjectName: row.SubjectName,
		ClassID:     uuidString(row.ClassID),
		ClassName:   row.ClassName,
		IsActive:    row.IsActive,
		CreatedAt:   timestamp(row.CreatedAt),
		UpdatedAt:   timestamp(row.UpdatedAt),
	}
}
