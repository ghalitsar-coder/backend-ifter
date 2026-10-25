package studentaccounts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// parent.go implements the guardian surface added on top of the student-account
// links (migration 000019):
//
//	admin (groups.Admin)   GET /users/:id/children
//	                       PUT /users/:id/children
//	parent (groups.Parent) GET /parent/children
//	                       GET /parent/children/:id/attendance
//	                       GET /parent/children/:id/attendance-summary
//	                       GET /parent/children/:id/discipline
//	                       GET /parent/children/:id/discipline-summary
//	                       GET /parent/children/:id/dispensations
//
// Every child read first resolves parent_students(user_id, :id); a student that
// is not linked to the caller answers 404 (ErrChildNotLinked) so an unrelated
// id is never confirmed to exist. The attendance/discipline aggregates reuse
// the same shared queries as the student's own portal, with the child's id
// forced after that check.

// ParentChildren lists the students linked to the signed-in guardian.
func (s *Service) ParentChildren(ctx context.Context, actor Actor) (ParentChildList, error) {
	userID, err := s.parentActorID(actor)
	if err != nil {
		return ParentChildList{}, err
	}
	rows, err := s.queries.ListParentLinks(ctx, userID)
	if err != nil {
		return ParentChildList{}, fmt.Errorf("list parent children: %w", err)
	}
	return ParentChildList{Data: parentChildrenOfLinks(rows)}, nil
}

// ParentAttendance is GET /parent/children/:id/attendance.
func (s *Service) ParentAttendance(ctx context.Context, actor Actor, childID string, filter DateRangeFilter) (MyAttendanceList, error) {
	child, err := s.parentChild(ctx, actor, childID)
	if err != nil {
		return MyAttendanceList{}, err
	}
	return s.attendanceListFor(ctx, child.StudentID, filter)
}

// ParentAttendanceSummary is GET /parent/children/:id/attendance-summary.
func (s *Service) ParentAttendanceSummary(ctx context.Context, actor Actor, childID string, filter DateRangeFilter) (MyAttendanceSummary, error) {
	child, err := s.parentChild(ctx, actor, childID)
	if err != nil {
		return MyAttendanceSummary{}, err
	}
	return s.attendanceSummaryFor(ctx, child.StudentID, filter)
}

// ParentDiscipline is GET /parent/children/:id/discipline.
func (s *Service) ParentDiscipline(ctx context.Context, actor Actor, childID string, filter DateRangeFilter) (MyDisciplineList, error) {
	child, err := s.parentChild(ctx, actor, childID)
	if err != nil {
		return MyDisciplineList{}, err
	}
	return s.disciplineListFor(ctx, child.StudentID, filter)
}

// ParentDisciplineSummary is GET /parent/children/:id/discipline-summary.
func (s *Service) ParentDisciplineSummary(ctx context.Context, actor Actor, childID string, filter DateRangeFilter) (MyDisciplineSummary, error) {
	child, err := s.parentChild(ctx, actor, childID)
	if err != nil {
		return MyDisciplineSummary{}, err
	}
	return s.disciplineSummaryFor(ctx, child.StudentID, child.StudentNumber, child.FullName, filter)
}

// ParentDispensations is GET /parent/children/:id/dispensations.
func (s *Service) ParentDispensations(ctx context.Context, actor Actor, childID string, limit, offset int32) (ParentDispensationList, error) {
	child, err := s.parentChild(ctx, actor, childID)
	if err != nil {
		return ParentDispensationList{}, err
	}
	limit, offset = normalizePage(limit, offset)
	rows, err := s.queries.ListDispensations(ctx, db.ListDispensationsParams{
		StudentID:  child.StudentID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return ParentDispensationList{}, fmt.Errorf("list child dispensations: %w", err)
	}
	total, err := s.queries.CountDispensations(ctx, db.CountDispensationsParams{
		StudentID: child.StudentID,
	})
	if err != nil {
		return ParentDispensationList{}, fmt.Errorf("count child dispensations: %w", err)
	}
	items := make([]ParentDispensation, 0, len(rows))
	for _, row := range rows {
		items = append(items, ParentDispensation{
			ID:        uuidString(row.ID),
			Type:      row.Type,
			Reason:    row.Reason,
			StartDate: dateString(row.StartDate),
			EndDate:   datePtr(row.EndDate),
			Status:    row.Status,
			Note:      textPtr(row.Note),
			CreatedAt: timestamp(row.CreatedAt),
		})
	}
	return ParentDispensationList{Data: items, Total: total, Limit: limit, Offset: offset}, nil
}

// AdminListParentLinks is GET /users/:id/children (admin only).
func (s *Service) AdminListParentLinks(ctx context.Context, actor Actor, userIDValue string) (ParentChildList, error) {
	if err := requireProvisionActor(actor); err != nil {
		return ParentChildList{}, err
	}
	userID, err := parseUUID(userIDValue)
	if err != nil {
		return ParentChildList{}, ErrInvalidInput
	}
	if _, err := s.queries.GetUserByID(ctx, userID); err != nil {
		if database.IsNotFound(err) {
			return ParentChildList{}, ErrNotFound
		}
		return ParentChildList{}, fmt.Errorf("get parent user: %w", err)
	}
	rows, err := s.queries.ListParentLinks(ctx, userID)
	if err != nil {
		return ParentChildList{}, fmt.Errorf("list parent links: %w", err)
	}
	return ParentChildList{Data: parentChildrenOfLinks(rows)}, nil
}

// AdminSetParentLinks is PUT /users/:id/children: it replaces the whole link
// set in one transaction and audits the before/after pairs. The target user
// must already carry the parent role; students of any status may be linked (a
// guardian keeps reading the history of a graduated child).
func (s *Service) AdminSetParentLinks(ctx context.Context, actor Actor, userIDValue string, input []ParentLinkInput, requestID string) (ParentChildList, error) {
	if err := requireProvisionActor(actor); err != nil {
		return ParentChildList{}, err
	}
	actorID, err := parseUUID(actor.ID)
	if err != nil {
		return ParentChildList{}, ErrUnauthorized
	}
	userID, err := parseUUID(userIDValue)
	if err != nil {
		return ParentChildList{}, ErrInvalidInput
	}
	if len(input) > maxParentChildren {
		return ParentChildList{}, ErrInvalidInput
	}
	if _, err := s.queries.GetUserByID(ctx, userID); err != nil {
		if database.IsNotFound(err) {
			return ParentChildList{}, ErrNotFound
		}
		return ParentChildList{}, fmt.Errorf("get parent user: %w", err)
	}
	roles, err := s.queries.ListUserRoles(ctx, userID)
	if err != nil {
		return ParentChildList{}, fmt.Errorf("list parent user roles: %w", err)
	}
	if !hasRoleCodes(roles, RoleParent) {
		return ParentChildList{}, ErrParentRoleRequired
	}
	links, err := s.normalizeParentLinks(ctx, input)
	if err != nil {
		return ParentChildList{}, err
	}

	beforeRows, err := s.queries.ListParentLinks(ctx, userID)
	if err != nil {
		return ParentChildList{}, fmt.Errorf("list parent links before update: %w", err)
	}
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		if err := q.DeleteParentLinksForUser(ctx, userID); err != nil {
			return fmt.Errorf("clear parent links: %w", err)
		}
		for _, link := range links {
			if err := q.InsertParentLink(ctx, db.InsertParentLinkParams{
				UserID:       userID,
				StudentID:    link.StudentID,
				Relationship: link.Relationship,
			}); err != nil {
				return fmt.Errorf("insert parent link: %w", err)
			}
		}
		afterRows, err := q.ListParentLinks(ctx, userID)
		if err != nil {
			return fmt.Errorf("list parent links after update: %w", err)
		}
		return s.auditParentLinks(ctx, q, actorID, userID,
			parentLinksAudit(beforeRows), parentLinksAudit(afterRows), requestID)
	})
	if err != nil {
		return ParentChildList{}, err
	}
	afterRows, err := s.queries.ListParentLinks(ctx, userID)
	if err != nil {
		return ParentChildList{}, fmt.Errorf("list parent links: %w", err)
	}
	return ParentChildList{Data: parentChildrenOfLinks(afterRows)}, nil
}

// normalizeParentLinks validates and de-duplicates one PUT payload. Duplicate
// student ids are rejected (the caller sends a set, not a sequence); an
// unknown student is a 404, a malformed id or relationship a 400.
func (s *Service) normalizeParentLinks(ctx context.Context, input []ParentLinkInput) ([]db.InsertParentLinkParams, error) {
	links := make([]db.InsertParentLinkParams, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, item := range input {
		studentID, err := parseUUID(item.StudentID)
		if err != nil {
			return nil, ErrInvalidInput
		}
		key := uuidString(studentID)
		if _, exists := seen[key]; exists {
			return nil, ErrInvalidInput
		}
		seen[key] = struct{}{}
		relationship, err := normalizeRelationship(item.Relationship)
		if err != nil {
			return nil, err
		}
		if _, err := s.queries.GetStudent(ctx, studentID); err != nil {
			if database.IsNotFound(err) {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("get linked student: %w", err)
		}
		links = append(links, db.InsertParentLinkParams{
			StudentID:    studentID,
			Relationship: relationship,
		})
	}
	return links, nil
}

func normalizeRelationship(value string) (pgtype.Text, error) {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "":
		return pgtype.Text{}, nil
	case RelationshipFather, RelationshipMother, RelationshipGuardian:
		return pgtype.Text{String: trimmed, Valid: true}, nil
	default:
		return pgtype.Text{}, ErrInvalidInput
	}
}

// parentChild resolves and authorizes one linked student for the parent routes.
func (s *Service) parentChild(ctx context.Context, actor Actor, childIDValue string) (db.GetParentChildRow, error) {
	userID, err := s.parentActorID(actor)
	if err != nil {
		return db.GetParentChildRow{}, err
	}
	childID, err := parseUUID(childIDValue)
	if err != nil {
		return db.GetParentChildRow{}, ErrInvalidInput
	}
	row, err := s.queries.GetParentChild(ctx, db.GetParentChildParams{UserID: userID, StudentID: childID})
	if err != nil {
		if database.IsNotFound(err) {
			return db.GetParentChildRow{}, ErrChildNotLinked
		}
		return db.GetParentChildRow{}, fmt.Errorf("get linked child: %w", err)
	}
	return row, nil
}

func (s *Service) parentActorID(actor Actor) (pgtype.UUID, error) {
	if strings.TrimSpace(actor.ID) == "" {
		return pgtype.UUID{}, ErrUnauthorized
	}
	if !hasRole(actor.Roles, RoleParent) {
		return pgtype.UUID{}, ErrForbidden
	}
	id, err := parseUUID(actor.ID)
	if err != nil {
		return pgtype.UUID{}, ErrUnauthorized
	}
	return id, nil
}

func hasRoleCodes(roles []db.Role, code string) bool {
	for _, role := range roles {
		if role.Code == code {
			return true
		}
	}
	return false
}

func parentChildrenOfLinks(rows []db.ListParentLinksRow) []ParentChild {
	children := make([]ParentChild, 0, len(rows))
	for _, row := range rows {
		children = append(children, ParentChild{
			StudentID:     uuidString(row.StudentID),
			StudentNumber: row.StudentNumber,
			FullName:      row.FullName,
			Status:        row.Status,
			ClassID:       uuidPtr(row.CurrentClassID),
			ClassName:     nullableString(row.CurrentClassName),
			Relationship:  textPtr(row.Relationship),
			LinkedAt:      timestamp(row.LinkedAt),
		})
	}
	return children
}

func parentLinksAudit(rows []db.ListParentLinksRow) []map[string]any {
	links := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		link := map[string]any{"student_id": uuidString(row.StudentID)}
		if row.Relationship.Valid {
			link["relationship"] = row.Relationship.String
		}
		links = append(links, link)
	}
	return links
}

func (s *Service) auditParentLinks(ctx context.Context, q *db.Queries, actorID, userID pgtype.UUID, before, after []map[string]any, requestID string) error {
	beforeData, err := json.Marshal(map[string]any{"children": before})
	if err != nil {
		return fmt.Errorf("encode parent links audit before data: %w", err)
	}
	afterData, err := json.Marshal(map[string]any{"children": after})
	if err != nil {
		return fmt.Errorf("encode parent links audit after data: %w", err)
	}
	if _, err := q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorUserID: actorID,
		Action:      "user.children.update",
		EntityType:  "user",
		EntityID:    userID,
		RequestID:   optionalText(requestID),
		BeforeData:  string(beforeData),
		AfterData:   string(afterData),
	}); err != nil {
		return fmt.Errorf("create parent links audit log: %w", err)
	}
	return nil
}

func datePtr(value pgtype.Date) *string {
	if !value.Valid {
		return nil
	}
	text := value.Time.Format("2006-01-02")
	return &text
}
