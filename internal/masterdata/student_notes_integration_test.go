package masterdata_test

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

type studentNoteBody struct {
	ID            string    `json:"id"`
	StudentID     string    `json:"student_id"`
	Note          string    `json:"note"`
	CreatedBy     string    `json:"created_by"`
	CreatedByName string    `json:"created_by_name"`
	CreatedAt     time.Time `json:"created_at"`
}

// TestStudentNotesAPI covers FR-C03: staff list/append/delete administrative
// student notes, teachers are refused by the MasterData role guard, and the
// note body is validated (non-blank, <= 2000 characters).
func TestStudentNotesAPI(t *testing.T) {
	env := newAPIEnv(t)
	adminID := env.insertUser(t, "note-admin", "note-admin@example.com", "active", "note-admin-password", "admin")
	staffID := env.insertUser(t, "note-staff", "note-staff@example.com", "active", "note-staff-password", "staff_tu")
	env.insertUser(t, "note-teacher", "note-teacher@example.com", "active", "note-teacher-password", "teacher")
	adminToken := env.login(t, "note-admin", "note-admin-password")
	staffToken := env.login(t, "note-staff", "note-staff-password")
	teacherToken := env.login(t, "note-teacher", "note-teacher-password")

	rec := env.request(t, http.MethodPost, "/api/v1/students", map[string]any{
		"student_number": "NIS-NOTE-1", "full_name": "Budi Notes", "admission_year": 2026, "status": "active",
	}, adminToken, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create student status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var createdStudent studentBody
	decodeField(t, rec, "student", &createdStudent)
	if createdStudent.ID == "" {
		t.Fatal("created student has no id")
	}
	studentPath := "/api/v1/students/" + createdStudent.ID + "/notes"

	t.Run("rbac", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, studentPath, nil, "", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, studentPath, nil, teacherToken, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, studentPath, map[string]any{"note": "nope"}, teacherToken, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodDelete, studentPath+"/00000000-0000-0000-0000-000000000000", nil, teacherToken, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher delete status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, studentPath, nil, staffToken, ""); rec.Code != http.StatusOK {
			t.Fatalf("staff_tu list status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("empty list", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, studentPath, nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data []studentNoteBody `json:"data"`
		}
		decode(t, rec, &body)
		if len(body.Data) != 0 {
			t.Fatalf("notes = %+v, want empty", body.Data)
		}
	})

	t.Run("unknown student", func(t *testing.T) {
		missing := "/api/v1/students/00000000-0000-0000-0000-0000000000ff/notes"
		if rec := env.request(t, http.MethodGet, missing, nil, adminToken, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, missing, map[string]any{"note": "hello"}, adminToken, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("validation", func(t *testing.T) {
		if rec := env.request(t, http.MethodPost, studentPath, map[string]any{}, adminToken, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("missing note status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, studentPath, map[string]any{"note": "   "}, adminToken, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("blank note status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, studentPath, map[string]any{"note": strings.Repeat("a", 2001)}, adminToken, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("too long note status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/students/not-a-uuid/notes", nil, adminToken, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid student id status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	var first studentNoteBody
	t.Run("create", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, studentPath, map[string]any{"note": "  Perlu konfirmasi wali kelas  "}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decodeField(t, rec, "note", &first)
		if first.ID == "" || first.StudentID != createdStudent.ID {
			t.Fatalf("created note = %+v", first)
		}
		if first.Note != "Perlu konfirmasi wali kelas" {
			t.Fatalf("note body = %q, want trimmed", first.Note)
		}
		if first.CreatedBy != adminID || first.CreatedByName != "Test note-admin" {
			t.Fatalf("created_by = %q (%q), want %q", first.CreatedBy, first.CreatedByName, adminID)
		}
		if first.CreatedAt.IsZero() {
			t.Fatal("created_at is zero")
		}

		// staff_tu may append notes too.
		rec = env.request(t, http.MethodPost, studentPath, map[string]any{"note": "Sudah dihubungi"}, staffToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("staff create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var second studentNoteBody
		decodeField(t, rec, "note", &second)
		if second.CreatedBy != staffID {
			t.Fatalf("staff created_by = %q, want %q", second.CreatedBy, staffID)
		}
	})

	t.Run("list newest first", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, studentPath, nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data []studentNoteBody `json:"data"`
		}
		decode(t, rec, &body)
		if len(body.Data) != 2 {
			t.Fatalf("notes = %d, want 2: %+v", len(body.Data), body.Data)
		}
		contents := map[string]bool{}
		for i, note := range body.Data {
			contents[note.Note] = true
			if i > 0 && note.CreatedAt.After(body.Data[i-1].CreatedAt) {
				t.Fatalf("notes are not newest-first: %+v", body.Data)
			}
		}
		if !contents["Perlu konfirmasi wali kelas"] || !contents["Sudah dihubungi"] {
			t.Fatalf("listed notes = %+v", body.Data)
		}
	})

	t.Run("delete", func(t *testing.T) {
		rec := env.request(t, http.MethodDelete, studentPath+"/"+first.ID, nil, staffToken, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodDelete, studentPath+"/"+first.ID, nil, adminToken, ""); rec.Code != http.StatusNotFound {
			t.Fatalf("second delete status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodDelete, studentPath+"/not-a-uuid", nil, adminToken, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid note id status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodGet, studentPath, nil, adminToken, "")
		var body struct {
			Data []studentNoteBody `json:"data"`
		}
		decode(t, rec, &body)
		if len(body.Data) != 1 || body.Data[0].Note != "Sudah dihubungi" {
			t.Fatalf("remaining notes = %+v", body.Data)
		}
	})

	t.Run("audit", func(t *testing.T) {
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'student_note.create'", 2)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'student_note.delete'", 1)
		assertCount(t, env.pool, `SELECT count(*) FROM audit_logs
			WHERE action = 'student_note.delete' AND before_data->>'note' = 'Perlu konfirmasi wali kelas'`, 1)
	})
}
