package imports_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
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

	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	importservice "github.com/ghalitsar-coder/backend-ifter/internal/imports"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool       *pgxpool.Pool
	router     http.Handler
	service    *importservice.Service
	storageDir string
}

type jobBody struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	TotalRows     int    `json:"total_rows"`
	ValidRows     int    `json:"valid_rows"`
	DuplicateRows int    `json:"duplicate_rows"`
	InvalidRows   int    `json:"invalid_rows"`
	SuccessRows   int    `json:"success_rows"`
}

type uploadBody struct {
	Job     jobBody `json:"job"`
	Preview []struct {
		RowNumber int    `json:"row_number"`
		Action    string `json:"action"`
		Key       string `json:"key"`
	} `json:"preview"`
	Errors []struct {
		RowNumber int    `json:"row_number"`
		Code      string `json:"code"`
	} `json:"errors"`
}

var (
	studentHeaders = []string{"NIS", "NISN", "Nama Lengkap", "Jenis Kelamin", "Tanggal Lahir", "Tahun Masuk", "Status", "Nama Wali", "Telepon Wali", "Tahun Ajaran", "Kelas", "Tanggal Mulai Kelas"}
	teacherHeaders = []string{"NIP", "Nama Lengkap", "Email", "Telepon", "Status"}
)

func TestImportExportAPI(t *testing.T) {
	env := newAPIEnv(t)
	env.insertUser(t, "import-admin", "import-admin@example.com", "active", "import-admin-password", "admin")
	env.insertUser(t, "import-teacher", "import-teacher@example.com", "active", "import-teacher-password", "teacher")
	adminToken := env.login(t, "import-admin", "import-admin-password")
	teacherToken := env.login(t, "import-teacher", "import-teacher-password")
	yearID := env.insertAcademicYear(t, "2026/2027", "2026-07-01", "2027-06-30")
	classID := env.insertClass(t, yearID, "10A", 10, "IPA")

	t.Run("rbac", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/imports", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/imports", nil, teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("student template and export", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/import-templates/students", nil, adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("template status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertSheetHeader(t, rec.Body.Bytes(), studentHeaders)
		assertTemplateExample(t, rec.Body.Bytes(), studentHeaders)
	})

	var studentJob jobBody
	t.Run("student import", func(t *testing.T) {
		data := workbook(t, studentHeaders, [][]string{
			{"1001", "", "Ani", "P", "2010-01-02", "2026", "active", "Wali Ani", "0812", "2026/2027", "10A", "2026-07-01"},
			{"1002", "", "Budi", "L", "", "2026", "", "", "", "", "", ""},
		})
		rec := env.upload(t, "students", "create_only", data, adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var uploaded uploadBody
		decode(t, rec, &uploaded)
		if uploaded.Job.Status != "validated" || uploaded.Job.TotalRows != 2 || uploaded.Job.ValidRows != 2 {
			t.Fatalf("uploaded job = %+v", uploaded.Job)
		}
		studentJob = uploaded.Job

		rec = env.confirm(t, studentJob.ID, adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("confirm status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var confirmed struct {
			Import jobBody `json:"import"`
		}
		decode(t, rec, &confirmed)
		if confirmed.Import.Status != "completed" || confirmed.Import.SuccessRows != 2 {
			t.Fatalf("confirmed job = %+v", confirmed.Import)
		}

		assertCount(t, env.pool, "SELECT count(*) FROM students WHERE student_number IN ('1001','1002')", 2)
		var name, guardian, status string
		var admissionYear int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT full_name, COALESCE(guardian_name,''), status, admission_year FROM students WHERE student_number = '1001'",
		).Scan(&name, &guardian, &status, &admissionYear); err != nil {
			t.Fatalf("query student: %v", err)
		}
		if name != "Ani" || guardian != "Wali Ani" || status != "active" || admissionYear != 2026 {
			t.Fatalf("student = %q %q %q %d", name, guardian, status, admissionYear)
		}
		assertCount(t, env.pool, "SELECT count(*) FROM class_enrollments e JOIN students s ON s.id=e.student_id WHERE s.student_number='1001' AND e.end_date IS NULL", 1)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action='import.complete'", 1)

		rec = env.request(t, http.MethodGet, "/api/v1/exports/students?status=active", nil, adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("export status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertExportContains(t, rec.Body.Bytes(), studentHeaders, "1001")
	})

	t.Run("upsert preserves blank cells", func(t *testing.T) {
		data := workbook(t, studentHeaders, [][]string{{"1001", "", "", "", "", "", "", "", "", "", "", ""}})
		rec := env.upload(t, "students", "upsert", data, adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var uploaded uploadBody
		decode(t, rec, &uploaded)
		if uploaded.Job.Status != "validated" || uploaded.Preview[0].Action != "unchanged" {
			t.Fatalf("uploaded job = %+v preview = %+v", uploaded.Job, uploaded.Preview)
		}
		if rec := env.confirm(t, uploaded.Job.ID, adminToken); rec.Code != http.StatusOK {
			t.Fatalf("confirm status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var name, guardian string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT full_name, COALESCE(guardian_name,'') FROM students WHERE student_number = '1001'",
		).Scan(&name, &guardian); err != nil {
			t.Fatalf("query student: %v", err)
		}
		if name != "Ani" || guardian != "Wali Ani" {
			t.Fatalf("blank cells overwrote student: %q %q", name, guardian)
		}
	})

	t.Run("duplicate blocks confirm", func(t *testing.T) {
		data := workbook(t, studentHeaders, [][]string{
			{"1001", "", "Ani", "P", "2010-01-02", "2026", "active", "Wali Ani", "0812", "2026/2027", "10A", "2026-07-01"},
		})
		rec := env.upload(t, "students", "create_only", data, adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var uploaded uploadBody
		decode(t, rec, &uploaded)
		if uploaded.Job.Status != "invalid" || uploaded.Job.DuplicateRows != 1 {
			t.Fatalf("uploaded job = %+v", uploaded.Job)
		}
		if len(uploaded.Errors) != 1 || uploaded.Errors[0].Code != "duplicate_nis" {
			t.Fatalf("errors = %+v", uploaded.Errors)
		}
		if rec := env.confirm(t, uploaded.Job.ID, adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("confirm status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("preview revalidates before apply", func(t *testing.T) {
		data := workbook(t, studentHeaders, [][]string{
			{"1003", "", "Citra", "P", "", "2026", "", "", "", "2026/2027", "10A", "2026-07-01"},
		})
		rec := env.upload(t, "students", "create_only", data, adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var uploaded uploadBody
		decode(t, rec, &uploaded)
		if uploaded.Job.Status != "validated" {
			t.Fatalf("uploaded job = %+v", uploaded.Job)
		}
		if _, err := env.pool.Exec(context.Background(),
			"INSERT INTO students (student_number, full_name, admission_year) VALUES ('1003','Direct Insert',2026)",
		); err != nil {
			t.Fatalf("insert direct student: %v", err)
		}
		rec = env.confirm(t, uploaded.Job.ID, adminToken)
		if rec.Code != http.StatusConflict {
			t.Fatalf("confirm status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Code string `json:"code"`
		}
		decode(t, rec, &body)
		if body.Code != "preview_changed" {
			t.Fatalf("code = %q", body.Code)
		}
		var status string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT status FROM import_jobs WHERE id = $1", uploaded.Job.ID,
		).Scan(&status); err != nil {
			t.Fatalf("query import job: %v", err)
		}
		if status != "invalid" {
			t.Fatalf("job status = %q", status)
		}
		assertCount(t, env.pool, "SELECT count(*) FROM students WHERE student_number='1003'", 1)
	})

	t.Run("all or nothing rollback", func(t *testing.T) {
		var existingID string
		if err := env.pool.QueryRow(context.Background(),
			"INSERT INTO students (student_number, full_name, admission_year) VALUES ('2001','Existing',2026) RETURNING id::text",
		).Scan(&existingID); err != nil {
			t.Fatalf("insert existing student: %v", err)
		}
		if _, err := env.pool.Exec(context.Background(),
			"INSERT INTO class_enrollments (student_id, class_id, start_date, end_date) VALUES ($1,$2,'2027-01-01','2027-02-01')",
			existingID, classID,
		); err != nil {
			t.Fatalf("insert future enrollment: %v", err)
		}
		data := workbook(t, studentHeaders, [][]string{
			{"2002", "", "Dua", "L", "", "2026", "", "", "", "2026/2027", "10A", "2026-07-01"},
			{"2001", "", "", "", "", "", "", "", "", "2026/2027", "10A", "2026-07-01"},
		})
		rec := env.upload(t, "students", "upsert", data, adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var uploaded uploadBody
		decode(t, rec, &uploaded)
		if uploaded.Job.Status != "validated" {
			t.Fatalf("uploaded job = %+v", uploaded.Job)
		}
		rec = env.confirm(t, uploaded.Job.ID, adminToken)
		if rec.Code != http.StatusConflict {
			t.Fatalf("confirm status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertCount(t, env.pool, "SELECT count(*) FROM students WHERE student_number='2002'", 0)
		var status string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT status FROM import_jobs WHERE id = $1", uploaded.Job.ID,
		).Scan(&status); err != nil {
			t.Fatalf("query import job: %v", err)
		}
		if status != "failed" {
			t.Fatalf("job status = %q", status)
		}
	})

	t.Run("teacher import", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/import-templates/teachers", nil, adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("template status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertSheetHeader(t, rec.Body.Bytes(), teacherHeaders)
		assertTemplateExample(t, rec.Body.Bytes(), teacherHeaders)

		data := workbook(t, teacherHeaders, [][]string{{"T001", "Guru Satu", "guru1@example.com", "0812", "active"}})
		rec = env.upload(t, "teachers", "create_only", data, adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var uploaded uploadBody
		decode(t, rec, &uploaded)
		if uploaded.Job.Status != "validated" {
			t.Fatalf("uploaded job = %+v", uploaded.Job)
		}
		if rec := env.confirm(t, uploaded.Job.ID, adminToken); rec.Code != http.StatusOK {
			t.Fatalf("confirm status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertCount(t, env.pool, "SELECT count(*) FROM teachers WHERE employee_number='T001'", 1)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action='import.complete'", 3)

		blank := workbook(t, teacherHeaders, [][]string{{"T001", "", "", "", ""}})
		rec = env.upload(t, "teachers", "upsert", blank, adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("upsert status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &uploaded)
		if uploaded.Job.Status != "validated" || uploaded.Preview[0].Action != "unchanged" {
			t.Fatalf("upsert job = %+v preview = %+v", uploaded.Job, uploaded.Preview)
		}
		if rec := env.confirm(t, uploaded.Job.ID, adminToken); rec.Code != http.StatusOK {
			t.Fatalf("upsert confirm status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var name, email string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT full_name, COALESCE(email,'') FROM teachers WHERE employee_number='T001'",
		).Scan(&name, &email); err != nil {
			t.Fatalf("query teacher: %v", err)
		}
		if name != "Guru Satu" || email != "guru1@example.com" {
			t.Fatalf("blank cells overwrote teacher: %q %q", name, email)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/exports/teachers", nil, adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("export status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertExportContains(t, rec.Body.Bytes(), teacherHeaders, "T001")
	})

	t.Run("file retention cleanup", func(t *testing.T) {
		if _, err := env.pool.Exec(context.Background(),
			"UPDATE import_jobs SET completed_at = now() - interval '2 hours' WHERE id = $1", studentJob.ID,
		); err != nil {
			t.Fatalf("age import job: %v", err)
		}
		if err := env.service.Cleanup(context.Background()); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
		var key string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT COALESCE(file_storage_key,'') FROM import_jobs WHERE id = $1", studentJob.ID,
		).Scan(&key); err != nil {
			t.Fatalf("query file key: %v", err)
		}
		if key != "" {
			t.Fatalf("file key not cleared: %q", key)
		}
		entries, err := os.ReadDir(env.storageDir)
		if err != nil {
			t.Fatalf("read storage dir: %v", err)
		}
		for _, entry := range entries {
			if entry.Name() == studentJob.ID+".xlsx" {
				t.Fatalf("expired import file still exists")
			}
		}
	})
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
	name := fmt.Sprintf("ifter_imports_test_%d", time.Now().UnixNano())
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
	storageDir := filepath.Join(t.TempDir(), "imports")
	service, err := importservice.NewService(pool, log, importservice.Config{
		Dir:            storageDir,
		MaxFileSize:    5 << 20,
		MaxRows:        100,
		FileRetention:  time.Hour,
		ErrorRetention: time.Hour,
	})
	if err != nil {
		t.Fatalf("create import service: %v", err)
	}
	router := httptransport.New(log, nil, authService, nil, service, nil, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router, service: service, storageDir: storageDir}
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

func (e apiEnv) upload(t *testing.T, entity, mode string, data []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "import.xlsx")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.WriteField("mode", mode); err != nil {
		t.Fatalf("write mode: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/"+entity, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e apiEnv) confirm(t *testing.T, jobID, token string) *httptest.ResponseRecorder {
	t.Helper()
	return e.request(t, http.MethodPost, "/api/v1/imports/"+jobID+"/confirm", nil, token)
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

func workbook(t *testing.T, headers []string, rows [][]string) []byte {
	t.Helper()
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	if err := file.SetSheetName("Sheet1", "Data"); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for index, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		if err := file.SetCellValue("Data", cell, header); err != nil {
			t.Fatalf("write header: %v", err)
		}
	}
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			if value == "" {
				continue
			}
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err := file.SetCellValue("Data", cell, value); err != nil {
				t.Fatalf("write cell: %v", err)
			}
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("write workbook: %v", err)
	}
	return buffer.Bytes()
}

func assertSheetHeader(t *testing.T, data []byte, want []string) {
	t.Helper()
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer func() { _ = file.Close() }()
	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		t.Fatal("workbook has no sheet")
	}
	rows, err := file.GetRows(sheets[0])
	if err != nil {
		t.Fatalf("read sheet: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("sheet has no header")
	}
	for index, header := range want {
		if index >= len(rows[0]) || rows[0][index] != header {
			t.Fatalf("header[%d] = %q, want %q", index, rows[0][index], header)
		}
	}
}

func assertTemplateExample(t *testing.T, data []byte, headers []string) {
	t.Helper()
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open template: %v", err)
	}
	defer func() { _ = file.Close() }()
	rows, err := file.GetRows("Contoh")
	if err != nil {
		t.Fatalf("read example sheet: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("example sheet has no example row: %+v", rows)
	}
	for index, header := range headers {
		if index >= len(rows[0]) || rows[0][index] != header {
			t.Fatalf("example header[%d] = %q, want %q", index, rows[0][index], header)
		}
	}
	if len(rows[1]) != len(headers) {
		t.Fatalf("example row has %d columns, want %d", len(rows[1]), len(headers))
	}
}

func assertExportContains(t *testing.T, data []byte, headers []string, value string) {
	t.Helper()
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer func() { _ = file.Close() }()
	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		t.Fatal("export has no sheet")
	}
	rows, err := file.GetRows(sheets[0])
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("export has no data rows: %+v", rows)
	}
	for index, header := range headers {
		if index >= len(rows[0]) || rows[0][index] != header {
			t.Fatalf("export header[%d] = %q, want %q", index, rows[0][index], header)
		}
	}
	found := false
	for _, row := range rows[1:] {
		for _, cell := range row {
			if cell == value {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("export does not contain %q", value)
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
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
