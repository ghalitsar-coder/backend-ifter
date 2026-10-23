package masterdata_test

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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/masterdata"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type academicYearBody struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	IsActive  bool   `json:"is_active"`
}

type termBody struct {
	ID             string `json:"id"`
	AcademicYearID string `json:"academic_year_id"`
	TermNumber     int16  `json:"term_number"`
	Name           string `json:"name"`
	StartDate      string `json:"start_date"`
	EndDate        string `json:"end_date"`
}

type classBody struct {
	ID                  string `json:"id"`
	AcademicYearID      string `json:"academic_year_id"`
	AcademicYearName    string `json:"academic_year_name"`
	Name                string `json:"name"`
	GradeLevel          int16  `json:"grade_level"`
	Major               string `json:"major"`
	Status              string `json:"status"`
	HomeroomTeacherID   string `json:"homeroom_teacher_id"`
	HomeroomTeacherName string `json:"homeroom_teacher_name"`
}

type teacherBody struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	EmployeeNumber string `json:"employee_number"`
	FullName       string `json:"full_name"`
	Status         string `json:"status"`
}

type studentBody struct {
	ID            string `json:"id"`
	StudentNumber string `json:"student_number"`
	FullName      string `json:"full_name"`
	Status        string `json:"status"`
	CurrentClass  *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"current_class"`
	Enrollments []struct {
		ClassID   string `json:"class_id"`
		ClassName string `json:"class_name"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	} `json:"enrollments"`
}

type homeroomBody struct {
	ID        string `json:"id"`
	TeacherID string `json:"teacher_id"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

func TestMasterDataAPI(t *testing.T) {
	env := newAPIEnv(t)
	env.insertUser(t, "md-admin", "md-admin@example.com", "active", "md-admin-password", "admin")
	teacherUserID := env.insertUser(t, "md-teacher", "md-teacher@example.com", "active", "md-teacher-password", "teacher")
	adminToken := env.login(t, "md-admin", "md-admin-password")
	teacherToken := env.login(t, "md-teacher", "md-teacher-password")

	t.Run("rbac", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/students", nil, "", ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/students", nil, teacherToken, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	ay1 := academicYearBody{}
	t.Run("academic years", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/academic-years", map[string]any{
			"name": "2026/2027", "start_date": "2026-07-01", "end_date": "2027-06-30", "is_active": true,
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create academic year status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decodeField(t, rec, "academic_year", &ay1)
		if !ay1.IsActive || ay1.Name != "2026/2027" {
			t.Fatalf("created academic year = %+v", ay1)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/academic-years", map[string]any{
			"name": "2027/2028", "start_date": "2027-07-01", "end_date": "2028-06-30", "is_active": true,
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create second academic year status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/academic-years", map[string]any{
			"name": "overlap", "start_date": "2027-01-01", "end_date": "2027-12-31",
		}, adminToken, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("overlap status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPut, "/api/v1/academic-years/"+ay1.ID, map[string]any{
			"name": "2026/2027", "start_date": "2026-07-01", "end_date": "2027-06-30", "is_active": true,
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("activate academic year status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var updated academicYearBody
		decodeField(t, rec, "academic_year", &updated)
		if !updated.IsActive {
			t.Fatalf("academic year was not activated: %+v", updated)
		}
		assertCount(t, env.pool, "SELECT count(*) FROM academic_years WHERE is_active", 1)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'academic_year.create'", 2)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'academic_year.update'", 1)
	})

	var termGanjil termBody
	t.Run("terms", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/academic-years/"+ay1.ID+"/terms", nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list terms status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var empty struct {
			Data []termBody `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &empty); err != nil {
			t.Fatalf("decode terms: %v", err)
		}
		if len(empty.Data) != 0 {
			t.Fatalf("expected no terms, got %+v", empty.Data)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/academic-years/"+ay1.ID+"/terms", map[string]any{
			"term_number": 1, "name": "Ganjil", "start_date": "2026-07-01", "end_date": "2026-12-31",
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create term status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decodeField(t, rec, "term", &termGanjil)

		rec = env.request(t, http.MethodPost, "/api/v1/academic-years/"+ay1.ID+"/terms", map[string]any{
			"term_number": 1, "name": "Duplikat", "start_date": "2027-01-01", "end_date": "2027-02-01",
		}, adminToken, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("duplicate term status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/academic-years/"+ay1.ID+"/terms", map[string]any{
			"term_number": 2, "name": "Genap", "start_date": "2026-12-01", "end_date": "2027-06-30",
		}, adminToken, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("overlapping term status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/academic-years/"+ay1.ID+"/terms", map[string]any{
			"term_number": 2, "name": "Luar", "start_date": "2027-01-01", "end_date": "2027-08-01",
		}, adminToken, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("out-of-year term status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPut, "/api/v1/terms/"+termGanjil.ID, map[string]any{
			"term_number": 1, "name": "Semester Ganjil", "start_date": "2026-07-01", "end_date": "2026-12-31",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("update term status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var updated termBody
		decodeField(t, rec, "term", &updated)
		if updated.Name != "Semester Ganjil" {
			t.Fatalf("updated term = %+v", updated)
		}

		if rec := env.request(t, http.MethodPost, "/api/v1/academic-years/"+ay1.ID+"/terms", map[string]any{
			"term_number": 2, "name": "Genap", "start_date": "2027-01-01", "end_date": "2027-06-30",
		}, teacherToken, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher create term status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'term.create'", 1)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'term.update'", 1)
	})

	classA := classBody{}
	classB := classBody{}
	t.Run("classes", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/classes", map[string]any{
			"academic_year_id": ay1.ID, "name": "10A", "grade_level": 10, "major": "IPA",
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create class A status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decodeField(t, rec, "class", &classA)

		rec = env.request(t, http.MethodPost, "/api/v1/classes", map[string]any{
			"academic_year_id": ay1.ID, "name": "10B", "grade_level": 10, "major": "IPS",
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create class B status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decodeField(t, rec, "class", &classB)

		rec = env.request(t, http.MethodPost, "/api/v1/classes", map[string]any{
			"academic_year_id": ay1.ID, "name": "10A", "grade_level": 10,
		}, adminToken, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("duplicate class status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodGet, "/api/v1/classes?academic_year_id="+ay1.ID, nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list classes status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list struct {
			Data  []classBody `json:"data"`
			Total int64       `json:"total"`
		}
		decode(t, rec, &list)
		if list.Total != 2 || len(list.Data) != 2 {
			t.Fatalf("class list = %+v", list)
		}

		rec = env.request(t, http.MethodPatch, "/api/v1/classes/"+classA.ID+"/status", map[string]string{
			"status": "archived",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("archive class status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var archived classBody
		decodeField(t, rec, "class", &archived)
		if archived.Status != "archived" {
			t.Fatalf("archived class = %+v", archived)
		}
		assertCount(t, env.pool, "SELECT count(*) FROM classes WHERE id = '"+classA.ID+"' AND status = 'archived'", 1)

		rec = env.request(t, http.MethodPatch, "/api/v1/classes/"+classA.ID+"/status", map[string]string{
			"status": "active",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("reactivate class status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	teacherA := teacherBody{}
	teacherB := teacherBody{}
	t.Run("teachers", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/teachers", map[string]any{
			"user_id": teacherUserID, "employee_number": "T001", "full_name": "Guru A", "email": "guru.a@example.com",
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create teacher A status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decodeField(t, rec, "teacher", &teacherA)
		if teacherA.UserID != teacherUserID || teacherA.EmployeeNumber != "T001" {
			t.Fatalf("created teacher A = %+v", teacherA)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/teachers", map[string]any{
			"employee_number": "T002", "full_name": "Guru B",
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create teacher B status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decodeField(t, rec, "teacher", &teacherB)

		rec = env.request(t, http.MethodPost, "/api/v1/teachers", map[string]any{
			"employee_number": "T002", "full_name": "Duplicate",
		}, adminToken, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("duplicate teacher status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodGet, "/api/v1/teachers?search=Guru&status=active", nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list teachers status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list struct {
			Data  []teacherBody `json:"data"`
			Total int64         `json:"total"`
		}
		decode(t, rec, &list)
		if list.Total != 2 {
			t.Fatalf("teacher list = %+v", list)
		}

		rec = env.request(t, http.MethodPut, "/api/v1/teachers/"+teacherB.ID, map[string]any{
			"employee_number": "T002", "full_name": "Guru B Updated",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("update teacher status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var updated teacherBody
		decodeField(t, rec, "teacher", &updated)
		if updated.FullName != "Guru B Updated" {
			t.Fatalf("updated teacher = %+v", updated)
		}

		rec = env.request(t, http.MethodPatch, "/api/v1/teachers/"+teacherB.ID+"/status", map[string]string{
			"status": "inactive",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("deactivate teacher status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPatch, "/api/v1/teachers/"+teacherB.ID+"/status", map[string]string{
			"status": "active",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("reactivate teacher status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("homeroom assignment", func(t *testing.T) {
		rec := env.request(t, http.MethodPut, "/api/v1/classes/"+classA.ID+"/homeroom", map[string]any{
			"teacher_id": teacherA.ID, "start_date": "2026-07-01",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("assign homeroom A status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var first homeroomBody
		decodeField(t, rec, "homeroom", &first)
		if first.TeacherID != teacherA.ID {
			t.Fatalf("first homeroom = %+v", first)
		}

		rec = env.request(t, http.MethodPut, "/api/v1/classes/"+classA.ID+"/homeroom", map[string]any{
			"teacher_id": teacherB.ID, "start_date": "2026-08-01",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("replace homeroom status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var second homeroomBody
		decodeField(t, rec, "homeroom", &second)
		if second.TeacherID != teacherB.ID || second.StartDate != "2026-08-01" {
			t.Fatalf("second homeroom = %+v", second)
		}
		assertCount(t, env.pool, "SELECT count(*) FROM homeroom_assignments WHERE class_id = '"+classA.ID+"' AND teacher_id = '"+teacherA.ID+"' AND end_date = '2026-07-31'", 1)

		rec = env.request(t, http.MethodGet, "/api/v1/classes/"+classA.ID, nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get class status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var detail struct {
			Class struct {
				HomeroomTeacherID   string `json:"homeroom_teacher_id"`
				HomeroomTeacherName string `json:"homeroom_teacher_name"`
			} `json:"class"`
		}
		decode(t, rec, &detail)
		if detail.Class.HomeroomTeacherID != teacherB.ID || detail.Class.HomeroomTeacherName != "Guru B Updated" {
			t.Fatalf("class homeroom detail = %+v", detail.Class)
		}

		rec = env.request(t, http.MethodPut, "/api/v1/classes/"+classB.ID+"/homeroom", map[string]any{
			"teacher_id": teacherB.ID, "start_date": "2026-09-01",
		}, adminToken, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("overlapping homeroom status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	studentA := studentBody{}
	t.Run("students and enrollment transfer", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/students", map[string]any{
			"student_number": "S001", "full_name": "Ahmad", "gender": "L",
			"admission_year": 2026, "class_id": classA.ID, "class_start_date": "2026-07-01",
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create student A status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decodeField(t, rec, "student", &studentA)
		if studentA.CurrentClass == nil || studentA.CurrentClass.ID != classA.ID {
			t.Fatalf("created student A = %+v", studentA)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/students", map[string]any{
			"student_number": "S002", "full_name": "Budi", "admission_year": 2026,
			"class_id": classA.ID, "class_start_date": "2026-07-02",
		}, adminToken, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create student B status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/students", map[string]any{
			"student_number": "S001", "full_name": "Duplicate", "admission_year": 2026,
		}, adminToken, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("duplicate student status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodGet, "/api/v1/students?search=Ahmad&class_id="+classA.ID+"&sort=nis", nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list students status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list struct {
			Data  []studentBody `json:"data"`
			Total int64         `json:"total"`
		}
		decode(t, rec, &list)
		if list.Total != 1 || len(list.Data) != 1 || list.Data[0].StudentNumber != "S001" {
			t.Fatalf("student search = %+v", list)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/students/"+studentA.ID, nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get student status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var detail studentBody
		decodeField(t, rec, "student", &detail)
		if len(detail.Enrollments) != 1 || detail.Enrollments[0].ClassID != classA.ID {
			t.Fatalf("student detail = %+v", detail)
		}

		rec = env.request(t, http.MethodPut, "/api/v1/students/"+studentA.ID, map[string]any{
			"student_number": "S001", "full_name": "Ahmad Updated", "gender": "L", "admission_year": 2026,
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("update student status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var updated studentBody
		decodeField(t, rec, "student", &updated)
		if updated.FullName != "Ahmad Updated" {
			t.Fatalf("updated student = %+v", updated)
		}

		rec = env.request(t, http.MethodPut, "/api/v1/students/"+studentA.ID+"/class", map[string]any{
			"class_id": classB.ID, "start_date": "2026-08-01",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("transfer student status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var transferred studentBody
		decodeField(t, rec, "student", &transferred)
		if transferred.CurrentClass == nil || transferred.CurrentClass.ID != classB.ID || len(transferred.Enrollments) != 2 {
			t.Fatalf("transferred student = %+v", transferred)
		}
		assertCount(t, env.pool, "SELECT count(*) FROM class_enrollments WHERE student_id = '"+studentA.ID+"' AND class_id = '"+classA.ID+"' AND end_date = '2026-07-31'", 1)

		rec = env.request(t, http.MethodGet, "/api/v1/classes/"+classA.ID+"/students", nil, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("class roster status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var roster struct {
			Data []studentBody `json:"data"`
		}
		decode(t, rec, &roster)
		if len(roster.Data) != 1 || roster.Data[0].StudentNumber != "S002" {
			t.Fatalf("class roster = %+v", roster)
		}

		rec = env.request(t, http.MethodPatch, "/api/v1/students/"+studentA.ID+"/status", map[string]string{
			"status": "inactive",
		}, adminToken, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("deactivate student status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertCount(t, env.pool, "SELECT count(*) FROM class_enrollments WHERE student_id = '"+studentA.ID+"' AND end_date IS NOT NULL", 2)
		rec = env.request(t, http.MethodPut, "/api/v1/students/"+studentA.ID+"/class", map[string]any{
			"class_id": classA.ID, "start_date": "2026-09-01",
		}, adminToken, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("assign inactive student status = %d, body = %s", rec.Code, rec.Body.String())
		}

		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'student.create'", 2)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'student.class_assign'", 1)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'student.status'", 1)
		assertCount(t, env.pool, `SELECT count(*) FROM audit_logs
			WHERE entity_type = 'student'
			  AND (after_data ? 'guardian_phone' OR after_data ? 'guardian_name' OR after_data ? 'birth_date')`, 0)

		rec = env.request(t, http.MethodPut, "/api/v1/academic-years/"+ay1.ID, map[string]any{
			"name": "2026/2027", "start_date": "2026-07-01", "end_date": "2026-10-01", "is_active": true,
		}, adminToken, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("shrink academic year status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
}

func TestMasterDataValidation(t *testing.T) {
	env := newAPIEnv(t)
	env.insertUser(t, "validation-admin", "", "active", "validation-admin-password", "admin")
	token := env.login(t, "validation-admin", "validation-admin-password")

	tests := []struct {
		method string
		path   string
		body   any
		want   int
	}{
		{http.MethodPost, "/api/v1/academic-years", map[string]any{"name": "bad", "start_date": "2026-07-01", "end_date": "2026-06-01"}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/classes", map[string]any{"academic_year_id": "not-a-uuid", "name": "10A", "grade_level": 10}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/teachers", map[string]any{"full_name": "Guru", "email": "not-an-email"}, http.StatusBadRequest},
		{http.MethodPost, "/api/v1/students", map[string]any{"student_number": "S001", "full_name": "Siswa", "admission_year": 1980}, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/students?sort=created_at", nil, http.StatusBadRequest},
		{http.MethodGet, "/api/v1/classes?limit=101", nil, http.StatusBadRequest},
	}
	for _, tt := range tests {
		rec := env.request(t, tt.method, tt.path, tt.body, token, "")
		if rec.Code != tt.want {
			t.Fatalf("%s %s = %d, want %d, body = %s", tt.method, tt.path, rec.Code, tt.want, rec.Body.String())
		}
	}
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

	name := fmt.Sprintf("ifter_masterdata_test_%d", time.Now().UnixNano())
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
	mdService := masterdata.NewService(pool, log)
	router := httptransport.New(log, nil, authService, mdService, nil, nil, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router}
}

func (e apiEnv) insertUser(t *testing.T, username, email, status, password string, roles ...string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var emailValue any
	if email != "" {
		emailValue = email
	}
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash, full_name, status)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
		username, emailValue, string(hash), "Test "+username, status,
	).Scan(&id); err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
	for _, role := range roles {
		if _, err := e.pool.Exec(context.Background(),
			"INSERT INTO user_roles (user_id, role_code) VALUES ($1, $2)", id, role,
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
	}, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	decode(t, rec, &body)
	return body.AccessToken
}

func (e apiEnv) request(t *testing.T, method, path string, body any, token, origin string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
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
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}

func decodeField(t *testing.T, rec *httptest.ResponseRecorder, field string, target any) {
	t.Helper()
	var body map[string]json.RawMessage
	decode(t, rec, &body)
	raw, ok := body[field]
	if !ok {
		t.Fatalf("response missing %q: %s", field, rec.Body.String())
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("decode field %q: %v", field, err)
	}
}

func assertCount(t *testing.T, pool *pgxpool.Pool, query string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(), query).Scan(&got); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
