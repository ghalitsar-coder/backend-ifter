package subjectattendance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	subjectattendancepkg "github.com/ghalitsar-coder/backend-ifter/internal/subjectattendance"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

// TestSubjectAttendanceAPI covers migration 000020 end to end: the subject
// catalog, teaching assignments, lesson sessions, record saving/closure and
// the recap report, including teacher ownership and RBAC.
func TestSubjectAttendanceAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	subjectPath := func(id string) string { return "/api/v1/subjects/" + id }
	assignmentPath := func(id string) string { return "/api/v1/teaching-assignments/" + id }
	sessionPath := func(id string) string { return "/api/v1/attendance-sessions/" + id }
	rangeQuery := "?from=2026-09-01&to=2026-09-30"

	var mathID, bioID, assignmentA, assignmentB, sessionID string

	t.Run("subject catalog RBAC and validation", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/subjects", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous list status = %d", rec.Code)
		}
		for name, token := range map[string]string{
			"admin":     data.adminToken,
			"staff_tu":  data.staffTUToken,
			"teacher":   data.teacherToken,
			"principal": data.principalToken,
		} {
			if rec := env.request(t, http.MethodGet, "/api/v1/subjects", nil, token); rec.Code != http.StatusOK {
				t.Fatalf("%s list status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/subjects", nil, data.studentToken); rec.Code != http.StatusForbidden {
			t.Fatalf("student list status = %d", rec.Code)
		}

		for name, token := range map[string]string{
			"teacher":   data.teacherToken,
			"principal": data.principalToken,
			"student":   data.studentToken,
		} {
			if rec := env.request(t, http.MethodPost, "/api/v1/subjects",
				map[string]any{"code": "X-" + name, "name": "X"}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s create status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}

		invalid := []struct {
			name string
			body any
			want int
		}{
			{"empty code", map[string]any{"code": "", "name": "Kosong"}, http.StatusBadRequest},
			{"blank code", map[string]any{"code": "   ", "name": "Kosong"}, http.StatusBadRequest},
			{"code with space", map[string]any{"code": "MTK 1", "name": "Kosong"}, http.StatusBadRequest},
			{"empty name", map[string]any{"code": "KOS", "name": ""}, http.StatusBadRequest},
			{"long name", map[string]any{"code": "KOS", "name": strings.Repeat("a", 151)}, http.StatusBadRequest},
			{"long description", map[string]any{"code": "KOS", "name": "Kosong", "description": strings.Repeat("a", 2001)}, http.StatusBadRequest},
			{"malformed json", nil, http.StatusBadRequest},
		}
		for _, testCase := range invalid {
			var rec *httptest.ResponseRecorder
			if testCase.body == nil {
				rec = env.requestRawString(t, http.MethodPost, "/api/v1/subjects", `{"code":`, data.adminToken)
			} else {
				rec = env.request(t, http.MethodPost, "/api/v1/subjects", testCase.body, data.adminToken)
			}
			if rec.Code != testCase.want {
				t.Fatalf("%s status = %d, want %d, body = %s", testCase.name, rec.Code, testCase.want, rec.Body.String())
			}
		}

		rec := env.request(t, http.MethodPost, "/api/v1/subjects",
			map[string]any{"code": "MTK", "name": "Matematika", "description": "Wajib"}, data.staffTUToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("staff_tu create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var math subjectEnvelope
		decode(t, rec, &math)
		mathID = math.Subject.ID
		if math.Subject.ID == "" || math.Subject.Code != "MTK" || math.Subject.Name != "Matematika" ||
			math.Subject.Description == nil || *math.Subject.Description != "Wajib" || !math.Subject.IsActive {
			t.Fatalf("created subject = %+v", math.Subject)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/subjects",
			map[string]any{"code": "BIO", "name": "Biologi"}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("admin create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var bio subjectEnvelope
		decode(t, rec, &bio)
		bioID = bio.Subject.ID

		if rec := env.request(t, http.MethodPost, "/api/v1/subjects",
			map[string]any{"code": "MTK", "name": "Duplikat"}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate code status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodGet, subjectPath(mathID), nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("get status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("cache control = %q, want no-store", got)
		}
		if rec := env.request(t, http.MethodGet, subjectPath("not-a-uuid"), nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed get status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, subjectPath("00000000-0000-4000-8000-000000000000"), nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown get status = %d", rec.Code)
		}

		// Omitting is_active on update must preserve the current value.
		rec = env.request(t, http.MethodPut, subjectPath(mathID),
			map[string]any{"code": "MTK", "name": "Matematika Wajib"}, data.staffTUToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var updated subjectEnvelope
		decode(t, rec, &updated)
		if updated.Subject.Name != "Matematika Wajib" || !updated.Subject.IsActive || updated.Subject.Description != nil {
			t.Fatalf("updated subject = %+v", updated.Subject)
		}

		rec = env.request(t, http.MethodPut, subjectPath(bioID),
			map[string]any{"code": "BIO", "name": "Biologi", "is_active": false}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("deactivate status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var deactivated subjectEnvelope
		decode(t, rec, &deactivated)
		if deactivated.Subject.IsActive {
			t.Fatalf("subject still active: %+v", deactivated.Subject)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/subjects?search=matematika", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("search status = %d", rec.Code)
		}
		var searched subjectattendancepkg.SubjectList
		decode(t, rec, &searched)
		if len(searched.Data) != 1 || searched.Data[0].ID != mathID {
			t.Fatalf("search result = %+v", searched.Data)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/subjects?is_active=false", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("filter status = %d", rec.Code)
		}
		var inactive subjectattendancepkg.SubjectList
		decode(t, rec, &inactive)
		if len(inactive.Data) != 1 || inactive.Data[0].ID != bioID {
			t.Fatalf("inactive list = %+v", inactive.Data)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/subjects?is_active=maybe", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid is_active status = %d", rec.Code)
		}
	})

	t.Run("teaching assignments RBAC, validation and scoping", func(t *testing.T) {
		create := func(teacherID, classID, token string) *httptest.ResponseRecorder {
			return env.request(t, http.MethodPost, "/api/v1/teaching-assignments", map[string]any{
				"teacher_id": teacherID,
				"subject_id": mathID,
				"class_id":   classID,
			}, token)
		}
		if rec := create(data.teacherID, data.classA, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := create(data.teacherID, data.classA, data.principalToken); rec.Code != http.StatusForbidden {
			t.Fatalf("principal create status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec := create(data.teacherID, data.classA, data.staffTUToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("staff_tu create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var first assignmentEnvelope
		decode(t, rec, &first)
		assignmentA = first.Assignment.ID
		if assignmentA == "" || first.Assignment.TeacherID != data.teacherID ||
			first.Assignment.TeacherName != "Budi Guru" || first.Assignment.SubjectCode != "MTK" ||
			first.Assignment.ClassName != "12A" || !first.Assignment.IsActive {
			t.Fatalf("created assignment = %+v", first.Assignment)
		}

		rec = create(data.teacher2ID, data.classB, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("second create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var second assignmentEnvelope
		decode(t, rec, &second)
		assignmentB = second.Assignment.ID

		if rec := create(data.teacherID, data.classA, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate triple status = %d, body = %s", rec.Code, rec.Body.String())
		}

		badRefs := []struct {
			name string
			body map[string]any
			want int
		}{
			{"malformed teacher", map[string]any{"teacher_id": "nope", "subject_id": mathID, "class_id": data.classA}, http.StatusBadRequest},
			{"unknown teacher", map[string]any{"teacher_id": "00000000-0000-4000-8000-000000000000", "subject_id": mathID, "class_id": data.classA}, http.StatusNotFound},
			{"unknown subject", map[string]any{"teacher_id": data.teacherID, "subject_id": "00000000-0000-4000-8000-000000000000", "class_id": data.classA}, http.StatusNotFound},
			{"unknown class", map[string]any{"teacher_id": data.teacherID, "subject_id": mathID, "class_id": "00000000-0000-4000-8000-000000000000"}, http.StatusNotFound},
		}
		for _, testCase := range badRefs {
			rec := env.request(t, http.MethodPost, "/api/v1/teaching-assignments", testCase.body, data.adminToken)
			if rec.Code != testCase.want {
				t.Fatalf("%s status = %d, want %d, body = %s", testCase.name, rec.Code, testCase.want, rec.Body.String())
			}
		}

		rec = env.request(t, http.MethodGet, "/api/v1/teaching-assignments", nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("teacher list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var own subjectattendancepkg.TeachingAssignmentList
		decode(t, rec, &own)
		if len(own.Data) != 1 || own.Data[0].ID != assignmentA {
			t.Fatalf("teacher assignments = %+v", own.Data)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/teaching-assignments", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin list status = %d", rec.Code)
		}
		var all subjectattendancepkg.TeachingAssignmentList
		decode(t, rec, &all)
		if len(all.Data) != 2 {
			t.Fatalf("admin assignments = %+v", all.Data)
		}

		if rec := env.request(t, http.MethodGet, assignmentPath(assignmentB), nil, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign assignment status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, assignmentPath("not-a-uuid"), nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed assignment status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, assignmentPath("00000000-0000-4000-8000-000000000000"), nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown assignment status = %d", rec.Code)
		}

		// Omitting is_active on update preserves the flag.
		rec = env.request(t, http.MethodPut, assignmentPath(assignmentA), map[string]any{
			"teacher_id": data.teacherID, "subject_id": mathID, "class_id": data.classA,
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("assignment update status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var updated assignmentEnvelope
		decode(t, rec, &updated)
		if !updated.Assignment.IsActive {
			t.Fatalf("assignment became inactive: %+v", updated.Assignment)
		}
	})

	t.Run("session lifecycle", func(t *testing.T) {
		createPayload := func(assignmentID, date string, start, end int) map[string]any {
			return map[string]any{
				"teaching_assignment_id": assignmentID,
				"session_date":           date,
				"period_start":           start,
				"period_end":             end,
				"topic":                  "Persamaan kuadrat",
			}
		}

		invalid := []struct {
			name string
			body map[string]any
			want int
		}{
			{"malformed assignment", createPayload("nope", "2026-09-10", 1, 2), http.StatusBadRequest},
			{"unknown assignment", createPayload("00000000-0000-4000-8000-000000000000", "2026-09-10", 1, 2), http.StatusNotFound},
			{"bad date", createPayload(assignmentA, "2026-13-01", 1, 2), http.StatusBadRequest},
			{"period below range", createPayload(assignmentA, "2026-09-10", 0, 2), http.StatusBadRequest},
			{"period inverted", createPayload(assignmentA, "2026-09-10", 3, 2), http.StatusBadRequest},
			{"period above range", createPayload(assignmentA, "2026-09-10", 1, 21), http.StatusBadRequest},
			{"long topic", createPayload(assignmentA, "2026-09-10", 1, 2), http.StatusBadRequest},
		}
		invalid[6].body["topic"] = strings.Repeat("a", 201)
		for _, testCase := range invalid {
			rec := env.request(t, http.MethodPost, "/api/v1/attendance-sessions", testCase.body, data.adminToken)
			if rec.Code != testCase.want {
				t.Fatalf("%s status = %d, want %d, body = %s", testCase.name, rec.Code, testCase.want, rec.Body.String())
			}
		}

		if rec := env.request(t, http.MethodPost, "/api/v1/attendance-sessions",
			createPayload(assignmentA, "2026-09-10", 1, 2), data.teacher2Token); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign teacher create status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec := env.request(t, http.MethodPost, "/api/v1/attendance-sessions",
			createPayload(assignmentA, "2026-09-10", 1, 2), data.teacherToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var created sessionEnvelope
		decode(t, rec, &created)
		sessionID = created.Session.ID
		if sessionID == "" || created.Session.TeachingAssignmentID != assignmentA ||
			created.Session.SessionDate != "2026-09-10" || created.Session.PeriodStart != 1 ||
			created.Session.PeriodEnd != 2 || created.Session.Status != "open" ||
			created.Session.Topic == nil || *created.Session.Topic != "Persamaan kuadrat" ||
			created.Session.TeacherName != "Budi Guru" || created.Session.SubjectCode != "MTK" ||
			created.Session.ClassName != "12A" || created.Session.ClosedAt != nil {
			t.Fatalf("created session = %+v", created.Session)
		}

		if rec := env.request(t, http.MethodPost, "/api/v1/attendance-sessions",
			createPayload(assignmentA, "2026-09-10", 1, 2), data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate session status = %d, body = %s", rec.Code, rec.Body.String())
		}

		if rec := env.request(t, http.MethodGet, "/api/v1/attendance-sessions", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("missing range status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance-sessions"+rangeQuery+"&status=done", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid status filter = %d", rec.Code)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/attendance-sessions"+rangeQuery, nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("teacher list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var teacherList subjectattendancepkg.SessionList
		decode(t, rec, &teacherList)
		if teacherList.Total != 1 || len(teacherList.Data) != 1 || teacherList.Data[0].ID != sessionID ||
			teacherList.Limit != 50 || teacherList.Offset != 0 {
			t.Fatalf("teacher sessions = %+v", teacherList)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/attendance-sessions"+rangeQuery, nil, data.teacher2Token)
		if rec.Code != http.StatusOK {
			t.Fatalf("other teacher list status = %d", rec.Code)
		}
		var otherList subjectattendancepkg.SessionList
		decode(t, rec, &otherList)
		if otherList.Total != 0 || len(otherList.Data) != 0 {
			t.Fatalf("other teacher sessions = %+v", otherList)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/attendance-sessions"+rangeQuery+"&teacher_id="+data.teacher2ID, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin filter status = %d", rec.Code)
		}
		var filtered subjectattendancepkg.SessionList
		decode(t, rec, &filtered)
		if filtered.Total != 0 {
			t.Fatalf("admin filter sessions = %+v", filtered)
		}

		if rec := env.request(t, http.MethodGet, sessionPath(sessionID), nil, data.teacher2Token); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign session detail status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, sessionPath(sessionID), nil, data.principalToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("principal detail status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var detail sessionEnvelope
		decode(t, rec, &detail)
		if detail.Session.ID != sessionID || detail.Session.Status != "open" {
			t.Fatalf("detail = %+v", detail.Session)
		}
		if rec := env.request(t, http.MethodGet, sessionPath("not-a-uuid"), nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed detail status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, sessionPath("00000000-0000-4000-8000-000000000000"), nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown detail status = %d", rec.Code)
		}

		rec = env.request(t, http.MethodGet, sessionPath(sessionID)+"/roster", nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("roster status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var roster subjectattendancepkg.Roster
		decode(t, rec, &roster)
		if len(roster.Data) != 3 {
			t.Fatalf("roster = %+v", roster.Data)
		}
		for _, row := range roster.Data {
			if row.Status != nil || row.Note != nil || row.EnrollmentID == "" {
				t.Fatalf("roster row before save = %+v", row)
			}
		}
		if rec := env.request(t, http.MethodGet, sessionPath(sessionID)+"/roster", nil, data.teacher2Token); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign roster status = %d", rec.Code)
		}

		recordsPayload := func(records ...map[string]any) map[string]any {
			return map[string]any{"records": records}
		}
		validRecords := recordsPayload(
			map[string]any{"student_id": data.studentA, "status": "present"},
			map[string]any{"student_id": data.studentB, "status": "present"},
			map[string]any{"student_id": data.studentC, "status": "late", "note": "Terlambat 10 menit"},
		)

		badRecords := []struct {
			name string
			body any
			want int
		}{
			{"empty records", recordsPayload(), http.StatusBadRequest},
			{"invalid status", recordsPayload(map[string]any{"student_id": data.studentA, "status": "unknown"}), http.StatusBadRequest},
			{"malformed student", recordsPayload(map[string]any{"student_id": "nope", "status": "present"}), http.StatusBadRequest},
			{"student outside roster", recordsPayload(map[string]any{"student_id": data.studentD, "status": "present"}), http.StatusBadRequest},
			{"duplicate student", recordsPayload(
				map[string]any{"student_id": data.studentA, "status": "present"},
				map[string]any{"student_id": data.studentA, "status": "absent"},
			), http.StatusBadRequest},
			{"long note", recordsPayload(map[string]any{
				"student_id": data.studentA, "status": "present", "note": strings.Repeat("a", 501),
			}), http.StatusBadRequest},
		}
		for _, testCase := range badRecords {
			rec := env.request(t, http.MethodPut, sessionPath(sessionID)+"/records", testCase.body, data.teacherToken)
			if rec.Code != testCase.want {
				t.Fatalf("%s status = %d, want %d, body = %s", testCase.name, rec.Code, testCase.want, rec.Body.String())
			}
		}
		if rec := env.request(t, http.MethodPut, sessionPath(sessionID)+"/records", validRecords, data.teacher2Token); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign save status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPut, sessionPath(sessionID)+"/records", validRecords, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var saved subjectattendancepkg.SessionRecordList
		decode(t, rec, &saved)
		if len(saved.Data) != 3 {
			t.Fatalf("saved records = %+v", saved.Data)
		}
		statuses := map[string]string{}
		for _, row := range saved.Data {
			statuses[row.StudentID] = row.Status
			if row.SessionID != sessionID || row.StudentNumber == "" || row.FullName == "" || row.EnrollmentID == "" {
				t.Fatalf("saved record = %+v", row)
			}
		}
		if statuses[data.studentA] != "present" || statuses[data.studentB] != "present" || statuses[data.studentC] != "late" {
			t.Fatalf("saved statuses = %+v", statuses)
		}

		rec = env.request(t, http.MethodGet, sessionPath(sessionID)+"/roster", nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("roster after save status = %d", rec.Code)
		}
		var rosterAfter subjectattendancepkg.Roster
		decode(t, rec, &rosterAfter)
		for _, row := range rosterAfter.Data {
			if row.Status == nil || *row.Status != statuses[row.StudentID] {
				t.Fatalf("roster after save row = %+v", row)
			}
		}

		rec = env.request(t, http.MethodGet, sessionPath(sessionID)+"/records", nil, data.principalToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("records read status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var listed subjectattendancepkg.SessionRecordList
		decode(t, rec, &listed)
		if len(listed.Data) != 3 {
			t.Fatalf("records read = %+v", listed.Data)
		}

		// Re-saving updates the existing rows instead of inserting duplicates.
		rec = env.request(t, http.MethodPut, sessionPath(sessionID)+"/records", recordsPayload(
			map[string]any{"student_id": data.studentC, "status": "excused", "note": "Surat izin"},
		), data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("resave status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var resaved subjectattendancepkg.SessionRecordList
		decode(t, rec, &resaved)
		if len(resaved.Data) != 3 {
			t.Fatalf("resaved records = %+v", resaved.Data)
		}
		for _, row := range resaved.Data {
			if row.StudentID == data.studentC && (row.Status != "excused" || row.Note == nil || *row.Note != "Surat izin") {
				t.Fatalf("updated record = %+v", row)
			}
		}

		rec = env.request(t, http.MethodGet, "/api/v1/attendance-sessions"+rangeQuery+"&status=open", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("open filter status = %d", rec.Code)
		}
		var openList subjectattendancepkg.SessionList
		decode(t, rec, &openList)
		if openList.Total != 1 {
			t.Fatalf("open sessions = %+v", openList)
		}

		if rec := env.request(t, http.MethodPost, sessionPath(sessionID)+"/close", nil, data.teacher2Token); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign close status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPost, sessionPath(sessionID)+"/close", nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("close status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var closed sessionEnvelope
		decode(t, rec, &closed)
		if closed.Session.Status != "closed" || closed.Session.ClosedAt == nil {
			t.Fatalf("closed session = %+v", closed.Session)
		}

		if rec := env.request(t, http.MethodPost, sessionPath(sessionID)+"/close", nil, data.teacherToken); rec.Code != http.StatusConflict {
			t.Fatalf("double close status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPut, sessionPath(sessionID)+"/records", validRecords, data.teacherToken); rec.Code != http.StatusConflict {
			t.Fatalf("closed save status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodGet, "/api/v1/attendance-sessions"+rangeQuery+"&status=closed", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("closed filter status = %d", rec.Code)
		}
		var closedList subjectattendancepkg.SessionList
		decode(t, rec, &closedList)
		if closedList.Total != 1 {
			t.Fatalf("closed sessions = %+v", closedList)
		}
	})

	t.Run("recap report", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/reports/subject-attendance", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("missing range status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/reports/subject-attendance"+rangeQuery, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous summary status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/reports/subject-attendance"+rangeQuery, nil, data.studentToken); rec.Code != http.StatusForbidden {
			t.Fatalf("student summary status = %d", rec.Code)
		}

		rec := env.request(t, http.MethodGet, "/api/v1/reports/subject-attendance"+rangeQuery, nil, data.staffTUToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("staff_tu summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var summary subjectattendancepkg.Summary
		decode(t, rec, &summary)
		if summary.From != "2026-09-01" || summary.To != "2026-09-30" || len(summary.Data) != 1 {
			t.Fatalf("summary = %+v", summary)
		}
		row := summary.Data[0]
		if row.TeachingAssignmentID != assignmentA || row.ClassName != "12A" || row.SubjectCode != "MTK" ||
			row.TeacherName != "Budi Guru" || row.Sessions != 1 || row.Records != 3 ||
			row.Present != 2 || row.Late != 0 || row.Excused != 1 || row.Absent != 0 ||
			row.Percentage != 66.67 {
			t.Fatalf("summary row = %+v", row)
		}
		if summary.Totals.Sessions != 1 || summary.Totals.Records != 3 ||
			summary.Totals.Present != 2 || summary.Totals.Excused != 1 || summary.Totals.Percentage != 66.67 {
			t.Fatalf("summary totals = %+v", summary.Totals)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/reports/subject-attendance"+rangeQuery, nil, data.teacher2Token)
		if rec.Code != http.StatusOK {
			t.Fatalf("other teacher summary status = %d", rec.Code)
		}
		var other subjectattendancepkg.Summary
		decode(t, rec, &other)
		if len(other.Data) != 0 || other.Totals.Sessions != 0 || other.Totals.Percentage != 0 {
			t.Fatalf("other teacher summary = %+v", other)
		}

		if rec := env.request(t, http.MethodGet, "/api/v1/reports/subject-attendance"+rangeQuery+"&class_id=nope", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed filter status = %d", rec.Code)
		}
	})

	t.Run("route RBAC and audit trail", func(t *testing.T) {
		for name, token := range map[string]string{
			"staff_tu":  data.staffTUToken,
			"principal": data.principalToken,
			"student":   data.studentToken,
		} {
			sessionCreate := map[string]any{
				"teaching_assignment_id": assignmentA,
				"session_date":           "2026-09-12",
				"period_start":           5,
				"period_end":             6,
			}
			if rec := env.request(t, http.MethodPost, "/api/v1/attendance-sessions", sessionCreate, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s session create status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}
		// Session reads are admin + teacher + duty_teacher + principal; staff_tu
		// and students stay out.
		for name, token := range map[string]string{
			"staff_tu": data.staffTUToken,
			"student":  data.studentToken,
		} {
			if rec := env.request(t, http.MethodGet, "/api/v1/attendance-sessions"+rangeQuery, nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s session list status = %d", name, rec.Code)
			}
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance-sessions"+rangeQuery, nil, data.principalToken); rec.Code != http.StatusOK {
			t.Fatalf("principal session list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// duty_teacher is a school-wide attendance operator.
		rec := env.request(t, http.MethodPost, "/api/v1/attendance-sessions", map[string]any{
			"teaching_assignment_id": assignmentA,
			"session_date":           "2026-09-12",
			"period_start":           5,
			"period_end":             6,
		}, data.dutyToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("duty create status = %d, body = %s", rec.Code, rec.Body.String())
		}

		// An inactive assignment accepts no new sessions.
		if rec := env.request(t, http.MethodPut, assignmentPath(assignmentB), map[string]any{
			"teacher_id": data.teacher2ID, "subject_id": mathID, "class_id": data.classB, "is_active": false,
		}, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("deactivate assignment status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/attendance-sessions", map[string]any{
			"teaching_assignment_id": assignmentB,
			"session_date":           "2026-09-12",
			"period_start":           1,
			"period_end":             2,
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("inactive assignment session status = %d, body = %s", rec.Code, rec.Body.String())
		}

		counts := map[string]int{}
		rows, err := env.pool.Query(context.Background(),
			`SELECT action, count(*) FROM audit_logs
			 WHERE action IN ('subject.create','subject.update','teaching_assignment.create',
			                  'teaching_assignment.update','attendance_session.create',
			                  'attendance_session.records','attendance_session.close')
			 GROUP BY action`)
		if err != nil {
			t.Fatalf("query audit logs: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var action string
			var count int
			if err := rows.Scan(&action, &count); err != nil {
				t.Fatalf("scan audit row: %v", err)
			}
			counts[action] = count
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate audit rows: %v", err)
		}
		want := map[string]int{
			"subject.create":             2,
			"subject.update":             2,
			"teaching_assignment.create": 2,
			"teaching_assignment.update": 2,
			"attendance_session.create":  2,
			"attendance_session.records": 2,
			"attendance_session.close":   1,
		}
		for action, count := range want {
			if counts[action] != count {
				t.Fatalf("audit %s = %d, want %d (all = %v)", action, counts[action], count, counts)
			}
		}
	})
}

type subjectEnvelope struct {
	Subject subjectattendancepkg.Subject `json:"subject"`
}

type assignmentEnvelope struct {
	Assignment subjectattendancepkg.TeachingAssignment `json:"assignment"`
}

type sessionEnvelope struct {
	Session subjectattendancepkg.Session `json:"session"`
}

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type testData struct {
	adminToken     string
	staffTUToken   string
	teacherToken   string
	teacher2Token  string
	principalToken string
	studentToken   string
	dutyToken      string
	teacherID      string
	teacher2ID     string
	classA         string
	classB         string
	studentA       string
	studentB       string
	studentC       string
	studentD       string
}

func newAPIEnv(t *testing.T) apiEnv {
	t.Helper()
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	adminConfig, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}
	name := fmt.Sprintf("ifter_subjectattendance_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		admin.Close()
		t.Fatalf("create test database: %v", err)
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		admin.Close()
	})
	testConfig, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}
	testConfig.ConnConfig.Database = name
	testConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err = pgxpool.NewWithConfig(ctx, testConfig)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	paths, err := filepath.Glob(filepath.Join("..", "..", "db", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatalf("find migrations: %v", err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read migration %s: %v", path, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
	log := discardLogger()
	tokens := authpkg.NewTokenManager("test-secret-at-least-32-characters-long", "test-issuer", 15*time.Minute)
	authService := authpkg.NewService(pool, tokens, authpkg.ServiceConfig{
		RefreshTokenTTL: 24 * time.Hour,
		InviteTokenTTL:  72 * time.Hour,
		ResetTokenTTL:   time.Hour,
		BcryptCost:      bcrypt.MinCost,
	}, log)
	router := httptransport.New(log, pool, authService, nil, nil, nil, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	e.insertUser(t, "sa-admin", "sa-admin@example.com", "sa-admin-pw", "admin")
	e.insertUser(t, "sa-staff", "sa-staff@example.com", "sa-staff-pw", "staff_tu")
	teacherUser := e.insertUser(t, "sa-teacher", "sa-teacher@example.com", "sa-teacher-pw", "teacher")
	teacher2User := e.insertUser(t, "sa-teacher2", "sa-teacher2@example.com", "sa-teacher2-pw", "teacher")
	e.insertUser(t, "sa-duty", "sa-duty@example.com", "sa-duty-pw", "duty_teacher")
	e.insertUser(t, "sa-principal", "sa-principal@example.com", "sa-principal-pw", "principal")
	e.insertUser(t, "sa-student", "sa-student@example.com", "sa-student-pw", "student")

	teacherID := e.insert(t,
		`INSERT INTO teachers (user_id, employee_number, full_name, status)
		 VALUES ($1, 'SA-T-001', 'Budi Guru', 'active') RETURNING id::text`, teacherUser)
	teacher2ID := e.insert(t,
		`INSERT INTO teachers (user_id, employee_number, full_name, status)
		 VALUES ($1, 'SA-T-002', 'Sari Guru', 'active') RETURNING id::text`, teacher2User)

	yearID := e.insert(t,
		`INSERT INTO academic_years (name, start_date, end_date, is_active)
		 VALUES ('2026/2027', '2026-07-01', '2027-06-30', true) RETURNING id::text`)
	classA := e.insert(t,
		`INSERT INTO classes (academic_year_id, name, grade_level) VALUES ($1, '12A', 12) RETURNING id::text`, yearID)
	classB := e.insert(t,
		`INSERT INTO classes (academic_year_id, name, grade_level) VALUES ($1, '12B', 12) RETURNING id::text`, yearID)

	studentA := e.insertStudent(t, "SA-001", "Ani Siswa", classA)
	studentB := e.insertStudent(t, "SA-002", "Budi Siswa", classA)
	studentC := e.insertStudent(t, "SA-003", "Citra Siswa", classA)
	studentD := e.insertStudent(t, "SA-004", "Dodi Siswa", classB)

	return testData{
		adminToken:     e.login(t, "sa-admin", "sa-admin-pw"),
		staffTUToken:   e.login(t, "sa-staff", "sa-staff-pw"),
		teacherToken:   e.login(t, "sa-teacher", "sa-teacher-pw"),
		teacher2Token:  e.login(t, "sa-teacher2", "sa-teacher2-pw"),
		principalToken: e.login(t, "sa-principal", "sa-principal-pw"),
		studentToken:   e.login(t, "sa-student", "sa-student-pw"),
		dutyToken:      e.login(t, "sa-duty", "sa-duty-pw"),
		teacherID:      teacherID,
		teacher2ID:     teacher2ID,
		classA:         classA,
		classB:         classB,
		studentA:       studentA,
		studentB:       studentB,
		studentC:       studentC,
		studentD:       studentD,
	}
}

func (e apiEnv) insertStudent(t *testing.T, number, name, classID string) string {
	t.Helper()
	studentID := e.insert(t,
		`INSERT INTO students (student_number, full_name, admission_year)
		 VALUES ($1, $2, 2024) RETURNING id::text`, number, name)
	e.insert(t,
		`INSERT INTO class_enrollments (student_id, class_id, start_date)
		 VALUES ($1, $2, '2026-07-01') RETURNING id::text`, studentID, classID)
	return studentID
}

func (e apiEnv) insert(t *testing.T, query string, args ...any) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&id); err != nil {
		t.Fatalf("insert %q: %v", query, err)
	}
	return id
}

func (e apiEnv) insertUser(t *testing.T, username, email, password string, roles ...string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash, full_name, status)
		 VALUES ($1, $2, $3, $4, 'active') RETURNING id::text`,
		username, email, string(hash), "Test "+username,
	).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", username, err)
	}
	for _, role := range roles {
		if _, err := e.pool.Exec(context.Background(),
			"INSERT INTO user_roles (user_id, role_code) VALUES ($1,$2)", id, role,
		); err != nil {
			t.Fatalf("assign role %q: %v", role, err)
		}
	}
	return id
}

func (e apiEnv) login(t *testing.T, username, password string) string {
	t.Helper()
	rec := e.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": username, "password": password,
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s status = %d, body = %s", username, rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	decode(t, rec, &body)
	return body.AccessToken
}

func (e apiEnv) request(t *testing.T, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e apiEnv) requestRawString(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
