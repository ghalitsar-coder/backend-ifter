package discipline_test

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
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"

	attendancepkg "github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	disciplinepkg "github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

const disciplineDay = "2026-09-02"

type apiEnv struct {
	pool    *pgxpool.Pool
	router  http.Handler
	service *disciplinepkg.Service
}

type testData struct {
	classID        string
	teacherID      string
	dutyID         string
	student1ID     string
	student2ID     string
	student3ID     string
	student4ID     string
	adminToken     string
	teacherToken   string
	dutyToken      string
	principalToken string
}

func TestDisciplineAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)
	lateCategoryID := env.categoryID(t, "late")

	var manualCategoryID string
	t.Run("categories", func(t *testing.T) {
		if rec := env.request(t, http.MethodPost, "/api/v1/discipline/categories", map[string]any{
			"name": "Tidak Berseragam", "default_points": 5,
		}, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher create category status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec := env.request(t, http.MethodPost, "/api/v1/discipline/categories", map[string]any{
			"code": "uniform", "name": "Tidak Berseragam", "description": "Pelanggaran seragam", "default_points": 5,
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create category status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var created disciplinepkg.Category
		decode(t, rec, &created)
		manualCategoryID = created.ID
		if created.DefaultPoints == nil || *created.DefaultPoints != 5 || !created.IsActive {
			t.Fatalf("created category = %+v", created)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/discipline/categories", map[string]any{
			"code": "uniform2", "name": "Tidak Berseragam",
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate category status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/discipline/categories?include_inactive=true", nil, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher inactive categories status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/discipline/categories", nil, data.teacherToken); rec.Code != http.StatusOK {
			t.Fatalf("teacher categories status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPatch, "/api/v1/discipline/categories/"+manualCategoryID+"/status", map[string]bool{"is_active": false}, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("deactivate category status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var teacherCategories struct {
			Data []disciplinepkg.Category `json:"data"`
		}
		rec = env.request(t, http.MethodGet, "/api/v1/discipline/categories", nil, data.teacherToken)
		decode(t, rec, &teacherCategories)
		for _, category := range teacherCategories.Data {
			if category.ID == manualCategoryID {
				t.Fatalf("inactive category visible to teacher: %+v", category)
			}
		}
		if rec := env.request(t, http.MethodPatch, "/api/v1/discipline/categories/"+manualCategoryID+"/status", map[string]bool{"is_active": true}, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("reactivate category status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPut, "/api/v1/discipline/categories/"+lateCategoryID, map[string]any{
			"code": "late", "name": "Terlambat", "description": "Keterlambatan berdasarkan absensi",
			"default_points": 2, "auto_on_attendance_status": "late",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("update late category status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/discipline/categories/"+lateCategoryID, map[string]any{
			"code": "late", "name": "Terlambat", "auto_on_attendance_status": "",
		}, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher update category status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	var manualRecordID string
	var dutyRecordID string
	t.Run("records and scope", func(t *testing.T) {
		if rec := env.request(t, http.MethodPost, "/api/v1/discipline/records", map[string]any{
			"student_id": data.student1ID, "category_id": manualCategoryID,
			"occurred_at": "2026-09-02T09:00:00+07:00", "description": "Tidak memakai dasi",
		}, data.principalToken); rec.Code != http.StatusForbidden {
			t.Fatalf("principal create record status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec := env.request(t, http.MethodPost, "/api/v1/discipline/records", map[string]any{
			"student_id": data.student1ID, "category_id": manualCategoryID,
			"occurred_at": "2026-09-02T09:00:00+07:00", "description": "Tidak memakai dasi",
		}, data.teacherToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("teacher create record status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var manual disciplinepkg.RecordDetail
		decode(t, rec, &manual)
		manualRecordID = manual.ID
		if manual.Points != 5 || manual.Status != "new" || manual.OccurredDate != disciplineDay {
			t.Fatalf("manual record = %+v", manual)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/discipline/records", map[string]any{
			"student_id": data.student1ID, "category_id": lateCategoryID,
			"occurred_at": "2026-09-02T09:00:00+07:00", "description": "Terlambat manual",
		}, data.teacherToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("manual auto-category status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/discipline/records", map[string]any{
			"student_id": data.student1ID, "category_id": manualCategoryID,
			"occurred_at": "2026-06-01T09:00:00+07:00", "description": "Di luar enrollment",
		}, data.teacherToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("outside enrollment status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/discipline/records", map[string]any{
			"student_id": data.student2ID, "category_id": manualCategoryID,
			"occurred_at": "2026-09-03T09:00:00+07:00", "description": "Bukan hari piket",
		}, data.dutyToken); rec.Code != http.StatusForbidden {
			t.Fatalf("duty off-day record status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPost, "/api/v1/discipline/records", map[string]any{
			"student_id": data.student2ID, "category_id": manualCategoryID,
			"occurred_at": "2026-09-02T10:00:00+07:00", "description": "Duty mencatat pelanggaran",
		}, data.dutyToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("duty create record status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var duty disciplinepkg.RecordDetail
		decode(t, rec, &duty)
		dutyRecordID = duty.ID

		if rec := env.request(t, http.MethodGet, "/api/v1/discipline/records", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous records status = %d", rec.Code)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/discipline/records?from=2026-09-01&to=2026-09-30", nil, data.principalToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("principal records status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list disciplinepkg.RecordList
		decode(t, rec, &list)
		if list.Total < 2 {
			t.Fatalf("principal records = %+v", list)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/discipline/records?limit=1&offset=0", nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("teacher records status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &list)
		if len(list.Data) != 1 || list.Total < 2 {
			t.Fatalf("paginated records = %+v", list)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/discipline/records?from=2026-09-30&to=2026-09-01", nil, data.teacherToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid date range status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/discipline/records?student_id="+data.student1ID, nil, data.dutyToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("duty records status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &list)
		for _, item := range list.Data {
			if item.OccurredDate != disciplineDay {
				t.Fatalf("duty saw off-day record: %+v", item)
			}
		}
	})

	t.Run("followups and points", func(t *testing.T) {
		if rec := env.request(t, http.MethodPost, "/api/v1/discipline/records/"+manualRecordID+"/followups", map[string]string{
			"status": "in_followup", "notes": "Dipanggil wali kelas",
		}, data.principalToken); rec.Code != http.StatusForbidden {
			t.Fatalf("principal followup status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec := env.request(t, http.MethodPost, "/api/v1/discipline/records/"+manualRecordID+"/followups", map[string]string{
			"status": "in_followup", "notes": "Dipanggil wali kelas",
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("followup status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var detail disciplinepkg.RecordDetail
		decode(t, rec, &detail)
		if detail.Status != "in_followup" || len(detail.Followups) != 1 {
			t.Fatalf("followup detail = %+v", detail)
		}
		if rec := env.request(t, http.MethodPatch, "/api/v1/discipline/records/"+manualRecordID+"/points", map[string]any{
			"points": 7, "reason": "Koreksi bobot",
		}, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher points status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPatch, "/api/v1/discipline/records/"+manualRecordID+"/points", map[string]any{
			"points": 7, "reason": "Koreksi bobot",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin points status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &detail)
		if detail.Points != 7 {
			t.Fatalf("points detail = %+v", detail)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/discipline/records/"+dutyRecordID+"/followups", map[string]string{
			"status": "cancelled",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("cancel without notes status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPost, "/api/v1/discipline/records/"+dutyRecordID+"/followups", map[string]string{
			"status": "cancelled", "notes": "Salah mencatat siswa",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("cancel status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &detail)
		if detail.Status != "cancelled" || len(detail.Followups) != 1 {
			t.Fatalf("cancelled detail = %+v", detail)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/discipline/students/"+data.student1ID, nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("teacher summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var summary disciplinepkg.StudentSummary
		decode(t, rec, &summary)
		if summary.TotalPoints != 7 || summary.TotalRecords != 1 || len(summary.Categories) != 1 {
			t.Fatalf("student summary = %+v", summary)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/discipline/students/"+data.student4ID, nil, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("out-of-scope summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("automatic late discipline", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     disciplineDay,
			"records": []map[string]string{
				{"student_id": data.student3ID, "status": "late", "check_in_at": "07:20"},
			},
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("late attendance status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/discipline/records?category_id="+lateCategoryID+"&student_id="+data.student3ID, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("auto late list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list disciplinepkg.RecordList
		decode(t, rec, &list)
		if list.Total != 1 || len(list.Data) != 1 {
			t.Fatalf("auto late records = %+v", list)
		}
		auto := list.Data[0]
		if auto.Points != 2 || auto.Status != "new" || auto.AttendanceRecordID == nil {
			t.Fatalf("auto late record = %+v", auto)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     disciplineDay,
			"records": []map[string]string{
				{"student_id": data.student3ID, "status": "present", "check_in_at": "07:05"},
			},
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("correct attendance status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/discipline/records/"+auto.ID, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("auto late detail status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var detail disciplinepkg.RecordDetail
		decode(t, rec, &detail)
		if detail.Status != "cancelled" || len(detail.Followups) != 1 {
			t.Fatalf("auto cancelled detail = %+v", detail)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     disciplineDay,
			"records": []map[string]string{
				{"student_id": data.student3ID, "status": "late", "check_in_at": "07:25"},
			},
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("late again status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/discipline/records?category_id="+lateCategoryID+"&student_id="+data.student3ID+"&status=new", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("active auto late list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &list)
		if list.Total != 1 || list.Data[0].ID == auto.ID {
			t.Fatalf("active auto late records = %+v", list)
		}
	})

	t.Run("export", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/exports/discipline?from=2026-09-01&to=2026-09-30", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("export status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if contentType := rec.Header().Get("Content-Type"); contentType == "" {
			t.Fatalf("export content type missing")
		}
		file, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("open export: %v", err)
		}
		defer func() { _ = file.Close() }()
		value, err := file.GetCellValue("Data Kedisiplinan", "A1")
		if err != nil || value != "NIS" {
			t.Fatalf("export header = %q, err = %v", value, err)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/exports/discipline?from=2026-09-02", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid export range status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("report", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/reports/discipline?from=2026-09-01&to=2026-09-30", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("discipline report status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var report disciplinepkg.Report
		decode(t, rec, &report)
		if report.TotalRecords != 4 || report.CancelledRecords != 2 || report.TotalPoints != 9 || len(report.Data) != 4 {
			t.Fatalf("discipline report = %+v", report)
		}
		statusCounts := make(map[string]int)
		for _, item := range report.ByStatus {
			statusCounts[item.Status] = item.Count
		}
		if statusCounts["new"] != 1 || statusCounts["in_followup"] != 1 || statusCounts["cancelled"] != 2 {
			t.Fatalf("discipline report status counts = %+v", report.ByStatus)
		}
		if len(report.ByCategory) != 2 {
			t.Fatalf("discipline report categories = %+v", report.ByCategory)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/reports/discipline?from=2026-09-01&to=2026-09-30", nil, data.principalToken); rec.Code != http.StatusOK {
			t.Fatalf("principal report status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/reports/discipline?from=2026-09-30&to=2026-09-01", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid report range status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var yearID string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT academic_year_id::text FROM classes WHERE id = $1", data.classID,
		).Scan(&yearID); err != nil {
			t.Fatalf("get class academic year: %v", err)
		}
		var termID string
		if err := env.pool.QueryRow(context.Background(),
			`INSERT INTO terms (academic_year_id, term_number, name, start_date, end_date)
			 VALUES ($1, 1, 'Ganjil Test', '2026-09-01', '2026-09-30') RETURNING id::text`,
			yearID,
		).Scan(&termID); err != nil {
			t.Fatalf("insert report term: %v", err)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/reports/discipline?term_id="+termID, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("term discipline report status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var termReport disciplinepkg.Report
		decode(t, rec, &termReport)
		if termReport.From != "2026-09-01" || termReport.To != "2026-09-30" || termReport.TotalRecords != 4 {
			t.Fatalf("term discipline report = %+v", termReport)
		}
	})
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	adminID := e.insertUser(t, "disc-admin", "disc-admin@example.com", "disc-admin-password", "admin")
	teacherID := e.insertUser(t, "disc-teacher", "disc-teacher@example.com", "disc-teacher-password", "teacher")
	dutyID := e.insertUser(t, "disc-duty", "disc-duty@example.com", "disc-duty-password", "duty_teacher")
	principalID := e.insertUser(t, "disc-principal", "disc-principal@example.com", "disc-principal-password", "principal")
	_ = adminID
	_ = principalID
	yearID := e.insertAcademicYear(t, "2026/2027", "2026-07-01", "2027-06-30")
	classID := e.insertClass(t, yearID, "10A", 10, "IPA")
	teacherProfileID := e.insertTeacher(t, teacherID, "Guru Wali")
	dutyProfileID := e.insertTeacher(t, dutyID, "Guru Piket")
	e.insertHomeroom(t, teacherProfileID, classID)
	e.insertDuty(t, dutyProfileID, 3)
	student1ID := e.insertStudent(t, "D001", "Ani")
	student2ID := e.insertStudent(t, "D002", "Budi")
	student3ID := e.insertStudent(t, "D003", "Citra")
	student4ID := e.insertStudent(t, "D004", "Dewi")
	e.insertEnrollment(t, student1ID, classID)
	e.insertEnrollment(t, student2ID, classID)
	e.insertEnrollment(t, student3ID, classID)
	return testData{
		classID:        classID,
		teacherID:      teacherProfileID,
		dutyID:         dutyProfileID,
		student1ID:     student1ID,
		student2ID:     student2ID,
		student3ID:     student3ID,
		student4ID:     student4ID,
		adminToken:     e.login(t, "disc-admin", "disc-admin-password"),
		teacherToken:   e.login(t, "disc-teacher", "disc-teacher-password"),
		dutyToken:      e.login(t, "disc-duty", "disc-duty-password"),
		principalToken: e.login(t, "disc-principal", "disc-principal-password"),
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
	name := fmt.Sprintf("ifter_discipline_test_%d", time.Now().UnixNano())
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
	attendanceService := attendancepkg.NewService(pool, log)
	service := disciplinepkg.NewService(pool, log)
	router := httptransport.New(log, nil, authService, nil, nil, attendanceService, service, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router, service: service}
}

func (e apiEnv) categoryID(t *testing.T, code string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT id::text FROM discipline_categories WHERE code = $1", code,
	).Scan(&id); err != nil {
		t.Fatalf("category %q: %v", code, err)
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
		t.Fatalf("insert user %q: %v", username, err)
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
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	decode(t, rec, &body)
	return body.AccessToken
}

func (e apiEnv) insertAcademicYear(t *testing.T, name, start, end string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO academic_years (name, start_date, end_date, is_active)
		 VALUES ($1,$2,$3,true) RETURNING id::text`, name, start, end,
	).Scan(&id); err != nil {
		t.Fatalf("insert academic year: %v", err)
	}
	return id
}

func (e apiEnv) insertClass(t *testing.T, yearID, name string, grade int, major string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO classes (academic_year_id, name, grade_level, major)
		 VALUES ($1,$2,$3,$4) RETURNING id::text`, yearID, name, grade, major,
	).Scan(&id); err != nil {
		t.Fatalf("insert class: %v", err)
	}
	return id
}

func (e apiEnv) insertTeacher(t *testing.T, userID, name string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO teachers (user_id, full_name) VALUES ($1,$2) RETURNING id::text`, userID, name,
	).Scan(&id); err != nil {
		t.Fatalf("insert teacher: %v", err)
	}
	return id
}

func (e apiEnv) insertHomeroom(t *testing.T, teacherID, classID string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO homeroom_assignments (teacher_id, class_id, start_date) VALUES ($1,$2,'2026-07-01')`,
		teacherID, classID,
	); err != nil {
		t.Fatalf("insert homeroom: %v", err)
	}
}

func (e apiEnv) insertDuty(t *testing.T, teacherID string, weekday int) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO duty_assignments (teacher_id, weekday, start_date) VALUES ($1,$2,'2026-07-01')`,
		teacherID, weekday,
	); err != nil {
		t.Fatalf("insert duty: %v", err)
	}
}

func (e apiEnv) insertStudent(t *testing.T, number, name string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO students (student_number, full_name, admission_year, status)
		 VALUES ($1,$2,2026,'active') RETURNING id::text`, number, name,
	).Scan(&id); err != nil {
		t.Fatalf("insert student: %v", err)
	}
	return id
}

func (e apiEnv) insertEnrollment(t *testing.T, studentID, classID string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO class_enrollments (student_id, class_id, start_date) VALUES ($1,$2,'2026-07-01')`,
		studentID, classID,
	); err != nil {
		t.Fatalf("insert enrollment: %v", err)
	}
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

func decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
