package employmenttype_test

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
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"

	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	employmenttypepkg "github.com/ghalitsar-coder/backend-ifter/internal/employmenttype"
	importservice "github.com/ghalitsar-coder/backend-ifter/internal/imports"
	"github.com/ghalitsar-coder/backend-ifter/internal/masterdata"
	"github.com/ghalitsar-coder/backend-ifter/internal/reporting"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type testData struct {
	adminToken   string
	staffToken   string
	teacherToken string
	studentToken string
	permanentID  string
	honoraryID   string
	staffTuID    string
}

func TestEmploymentTypeAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	decode := func(t *testing.T, rec *httptest.ResponseRecorder, target any) {
		t.Helper()
		if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
			t.Fatalf("decode %q: %v", rec.Body.String(), err)
		}
	}
	typeFromEnvelope := func(t *testing.T, rec *httptest.ResponseRecorder) employmenttypepkg.EmploymentType {
		t.Helper()
		var body struct {
			EmploymentType employmenttypepkg.EmploymentType `json:"employment_type"`
		}
		decode(t, rec, &body)
		if body.EmploymentType.ID == "" {
			t.Fatalf("missing employment_type envelope: %s", rec.Body.String())
		}
		return body.EmploymentType
	}

	t.Run("authentication and rbac", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/employment-types", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous list status = %d", rec.Code)
		}
		// MasterData = admin + staff_tu. teacher/student/principal are out.
		for name, token := range map[string]string{"teacher": data.teacherToken, "student": data.studentToken} {
			if rec := env.request(t, http.MethodGet, "/api/v1/employment-types", nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s list status = %d", name, rec.Code)
			}
			if rec := env.request(t, http.MethodPost, "/api/v1/employment-types", map[string]any{
				"code": "x", "name": "y",
			}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s create status = %d", name, rec.Code)
			}
		}
		for name, token := range map[string]string{"admin": data.adminToken, "staff_tu": data.staffToken} {
			if rec := env.request(t, http.MethodGet, "/api/v1/employment-types", nil, token); rec.Code != http.StatusOK {
				t.Fatalf("%s list status = %d", name, rec.Code)
			}
		}
	})

	t.Run("seeded contract categories", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/employment-types", nil, data.adminToken)
		var list employmenttypepkg.TypeList
		decode(t, rec, &list)
		byCode := map[string]employmenttypepkg.EmploymentType{}
		for _, item := range list.Data {
			byCode[item.Code] = item
		}
		for _, code := range []string{"permanent", "honorary", "staff_tu"} {
			item, ok := byCode[code]
			if !ok {
				t.Fatalf("seed code %q missing from %s", code, rec.Body.String())
			}
			if !item.IsActive || item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() {
				t.Fatalf("seed %q = %+v", code, item)
			}
		}
	})

	var contract string
	t.Run("employment-type lifecycle (staff_tu writes)", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/employment-types", map[string]any{
			"code": "contract", "name": "Kontrak", "description": "Guru kontrak",
		}, data.staffToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		created := typeFromEnvelope(t, rec)
		contract = created.ID
		if created.Code != "contract" || created.Name != "Kontrak" || !created.IsActive ||
			created.Description == nil || *created.Description != "Guru kontrak" {
			t.Fatalf("created = %+v", created)
		}
		// Duplicate code -> 409.
		if rec := env.request(t, http.MethodPost, "/api/v1/employment-types", map[string]any{
			"code": "contract", "name": "Kontrak dua",
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// GET by id envelope.
		rec = env.request(t, http.MethodGet, "/api/v1/employment-types/"+contract, nil, data.staffToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("get status = %d", rec.Code)
		}
		typeFromEnvelope(t, rec)
		// PUT: rename + explicit is_active=false; description omitted clears it.
		rec = env.request(t, http.MethodPut, "/api/v1/employment-types/"+contract, map[string]any{
			"code": "contract", "name": "Kontrak baru", "is_active": false,
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
		}
		updated := typeFromEnvelope(t, rec)
		if updated.Name != "Kontrak baru" || updated.IsActive || updated.Description != nil {
			t.Fatalf("updated = %+v", updated)
		}
		// PUT without is_active keeps it false (COALESCE).
		rec = env.request(t, http.MethodPut, "/api/v1/employment-types/"+contract, map[string]any{
			"code": "contract", "name": "Kontrak baru",
		}, data.adminToken)
		if updated = typeFromEnvelope(t, rec); updated.IsActive {
			t.Fatalf("omitted is_active must not flip the row: %+v", updated)
		}
		// PATCH status back to active.
		rec = env.request(t, http.MethodPatch, "/api/v1/employment-types/"+contract+"/status",
			map[string]bool{"is_active": true}, data.staffToken)
		if rec.Code != http.StatusOK || !typeFromEnvelope(t, rec).IsActive {
			t.Fatalf("reactivate status/body = %+v", rec)
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]any
		}{
			{"missing code", map[string]any{"name": "x"}},
			{"missing name", map[string]any{"code": "abc"}},
			{"uppercase code", map[string]any{"code": "ABC", "name": "x"}},
			{"dash in code", map[string]any{"code": "a-b", "name": "x"}},
			{"long code", map[string]any{"code": strings.Repeat("a", 31), "name": "x"}},
		}
		for _, tt := range cases {
			if rec := env.request(t, http.MethodPost, "/api/v1/employment-types", tt.body, data.adminToken); rec.Code != http.StatusBadRequest {
				t.Errorf("%s: status = %d, body = %s", tt.name, rec.Code, rec.Body.String())
			}
		}
		if rec := env.request(t, http.MethodPatch, "/api/v1/employment-types/"+contract+"/status", map[string]any{}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Errorf("missing is_active: status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/employment-types/not-a-uuid", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Errorf("bad id: status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/employment-types/"+pgNilUUID(), nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Errorf("unknown id: status = %d", rec.Code)
		}
	})

	t.Run("writes are audited", func(t *testing.T) {
		counts := map[string]int{}
		rows, err := env.pool.Query(context.Background(),
			"SELECT action FROM audit_logs WHERE entity_type = 'employment_type'")
		if err != nil {
			t.Fatalf("read audit rows: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var action string
			if err := rows.Scan(&action); err != nil {
				t.Fatalf("scan audit: %v", err)
			}
			counts[action]++
		}
		if counts["employment_type.create"] != 1 {
			t.Fatalf("create audit rows = %d, want 1", counts["employment_type.create"])
		}
		if counts["employment_type.update"] < 2 {
			t.Fatalf("update audit rows = %d, want >= 2", counts["employment_type.update"])
		}
		if counts["employment_type.status"] != 1 {
			t.Fatalf("status audit rows = %d, want 1", counts["employment_type.status"])
		}
	})

	t.Run("teacher dto carries employment type id and name", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/teachers", map[string]any{
			"full_name": "Guru Permanen", "employee_number": "EMP-TP-1",
			"employment_type_id": data.permanentID,
		}, data.staffToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create teacher status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var created struct {
			Teacher masterdata.Teacher `json:"teacher"`
		}
		decode(t, rec, &created)
		if created.Teacher.EmploymentTypeID != data.permanentID || created.Teacher.EmploymentTypeName != "Permanen (PNS/PPPK)" {
			t.Fatalf("teacher = %+v", created.Teacher)
		}
		// Update moves it to honorary; the response carries the new name.
		rec = env.request(t, http.MethodPut, "/api/v1/teachers/"+created.Teacher.ID, map[string]any{
			"full_name": "Guru Permanen", "employee_number": "EMP-TP-1",
			"employment_type_id": data.honoraryID,
		}, data.staffToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("update teacher status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &created)
		if created.Teacher.EmploymentTypeID != data.honoraryID || created.Teacher.EmploymentTypeName != "Honorer" {
			t.Fatalf("updated teacher = %+v", created.Teacher)
		}
		// Detail + list expose both fields.
		rec = env.request(t, http.MethodGet, "/api/v1/teachers/"+created.Teacher.ID, nil, data.adminToken)
		decode(t, rec, &created)
		if created.Teacher.EmploymentTypeName != "Honorer" {
			t.Fatalf("get teacher = %+v", created.Teacher)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/teachers?search=EMP-TP-1", nil, data.adminToken)
		var list struct {
			Data []masterdata.Teacher `json:"data"`
		}
		decode(t, rec, &list)
		if len(list.Data) != 1 || list.Data[0].EmploymentTypeName != "Honorer" {
			t.Fatalf("list = %+v", list.Data)
		}
		// A teacher without the field stays unknown: keys are omitted.
		rec = env.request(t, http.MethodPost, "/api/v1/teachers", map[string]any{
			"full_name": "Guru Lama", "employee_number": "EMP-TP-2",
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create plain teacher status = %d", rec.Code)
		}
		if strings.Contains(rec.Body.String(), `"employment_type_id"`) {
			t.Fatalf("plain teacher must omit employment_type_id: %s", rec.Body.String())
		}
		// Unknown/invalid ids are rejected as invalid input (400, FK/format).
		if rec := env.request(t, http.MethodPost, "/api/v1/teachers", map[string]any{
			"full_name": "x", "employment_type_id": pgNilUUID(),
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("dangling employment_type_id status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/teachers", map[string]any{
			"full_name": "x", "employment_type_id": "bukan-uuid",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed employment_type_id status = %d", rec.Code)
		}
	})

	t.Run("dashboard gains active_teachers_breakdown", func(t *testing.T) {
		before := env.dashboardBreakdown(t, data.adminToken)
		if before == nil {
			t.Fatal("dashboard missing active_teachers_breakdown")
		}
		// Add one teacher per category; unknown already exists (EMP-TP-2).
		for _, item := range []struct {
			nip, name, id string
		}{
			{"EMP-BR-1", "Buku Honorer", data.honoraryID},
			{"EMP-BR-2", "Buku TU", data.staffTuID},
		} {
			rec := env.request(t, http.MethodPost, "/api/v1/teachers", map[string]any{
				"full_name": item.name, "employee_number": item.nip, "employment_type_id": item.id,
			}, data.adminToken)
			if rec.Code != http.StatusCreated {
				t.Fatalf("create %s: %d %s", item.nip, rec.Code, rec.Body.String())
			}
		}
		after := env.dashboardBreakdown(t, data.adminToken)
		if after.Honorary != before.Honorary+1 || after.StaffTU != before.StaffTU+1 {
			t.Fatalf("breakdown after = %+v, before = %+v", after, before)
		}
		if after.Unknown < before.Unknown || after.Unknown < 1 {
			t.Fatalf("unknown bucket = %+v (EMP-TP-2 must land in unknown)", after.Unknown)
		}
		if sum := after.Permanent + after.Honorary + after.StaffTU + after.Unknown; sum > after.total() {
			t.Fatalf("breakdown buckets %d exceed active teacher count %d", sum, after.total())
		}
	})

	t.Run("teacher import template and export carry the column", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/import-templates/teachers", nil, data.staffToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("template status = %d", rec.Code)
		}
		headers := sheetHeader(t, rec.Body.Bytes())
		if len(headers) == 0 || headers[len(headers)-1] != "Jenis Kepegawaian" {
			t.Fatalf("template headers = %v", headers)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/exports/teachers", nil, data.staffToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("export status = %d", rec.Code)
		}
		file, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("open export: %v", err)
		}
		defer func() { _ = file.Close() }()
		rows, err := file.GetRows(file.GetSheetList()[0])
		if err != nil || len(rows) == 0 {
			t.Fatalf("export rows: %v (%d rows)", err, len(rows))
		}
		if rows[0][len(rows[0])-1] != "Jenis Kepegawaian" {
			t.Fatalf("export headers = %v", rows[0])
		}
		found := false
		for _, row := range rows[1:] {
			if len(row) >= 6 && row[0] == "EMP-TP-1" && row[5] == "Honorer" {
				found = true
			}
		}
		if !found {
			t.Fatalf("exported Honorer row for EMP-TP-1 missing: %v", rows)
		}
	})

	t.Run("teacher import accepts optional Jenis Kepegawaian column", func(t *testing.T) {
		// Row 1 maps Honorer; row 2 has an unknown value -> warning, still
		// imported without a type; the workbook header is otherwise standard.
		upload := env.uploadTeacherWorkbook(t, data.staffToken,
			[]string{"T-EMP-1", "Imported Honorer", "", "", "active", "Honorer"},
			[]string{"T-EMP-2", "Imported Alien", "", "", "active", "Alien"},
		)
		if upload.Job.Status != "validated" {
			t.Fatalf("warning-only errors must keep the job validated: %+v", upload.Job)
		}
		if upload.Job.ValidRows != 2 || upload.Job.InvalidRows != 0 {
			t.Fatalf("job counters = %+v", upload.Job)
		}
		warnings := 0
		for _, rowErr := range upload.Errors {
			if rowErr.Code == "warning" && rowErr.Column == "Jenis Kepegawaian" {
				warnings++
			}
		}
		if warnings != 1 {
			t.Fatalf("warning rows = %d, want 1: %+v", warnings, upload.Errors)
		}
		rec := env.request(t, http.MethodPost, "/api/v1/imports/"+upload.Job.ID+"/confirm", nil, data.staffToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("confirm status = %d, body = %s", rec.Code, rec.Body.String())
		}
		listRec := env.request(t, http.MethodGet, "/api/v1/teachers?search=T-EMP-1", nil, data.staffToken)
		var withType struct {
			Data []masterdata.Teacher `json:"data"`
		}
		decode(t, listRec, &withType)
		if len(withType.Data) != 1 || withType.Data[0].EmploymentTypeName != "Honorer" {
			t.Fatalf("imported T-EMP-1 = %+v", withType.Data)
		}
		listRec = env.request(t, http.MethodGet, "/api/v1/teachers?search=T-EMP-2", nil, data.staffToken)
		var withoutType struct {
			Data []masterdata.Teacher `json:"data"`
		}
		decode(t, listRec, &withoutType)
		if len(withoutType.Data) != 1 || withoutType.Data[0].EmploymentTypeID != "" {
			t.Fatalf("imported alien row must stay without a type: %+v", withoutType.Data)
		}
	})
}

func (e apiEnv) dashboardBreakdown(t *testing.T, token string) *breakdownRow {
	t.Helper()
	rec := e.request(t, http.MethodGet, "/api/v1/dashboard", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ActiveTeachers          *int32        `json:"active_teachers"`
		ActiveTeachersBreakdown *breakdownRow `json:"active_teachers_breakdown"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	return body.ActiveTeachersBreakdown
}

type breakdownRow struct {
	Permanent int32 `json:"permanent"`
	Honorary  int32 `json:"honorary"`
	StaffTU   int32 `json:"staff_tu"`
	Unknown   int32 `json:"unknown"`
	Total     *int32
}

func (b breakdownRow) total() int32 { return b.Permanent + b.Honorary + b.StaffTU + b.Unknown }

type teacherUploadResult struct {
	Job struct {
		ID          string `json:"id"`
		Status      string `json:"status"`
		ValidRows   int32  `json:"valid_rows"`
		InvalidRows int32  `json:"invalid_rows"`
	} `json:"job"`
	Preview []struct {
		RowNumber int    `json:"row_number"`
		Action    string `json:"action"`
	} `json:"preview"`
	Errors []struct {
		RowNumber int    `json:"row_number"`
		Column    string `json:"column"`
		Code      string `json:"code"`
	} `json:"errors"`
}

func (e apiEnv) uploadTeacherWorkbook(t *testing.T, token string, dataRows ...[]string) teacherUploadResult {
	t.Helper()
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	headers := []string{"NIP", "Nama Lengkap", "Email", "Telepon", "Status", "Jenis Kepegawaian"}
	for index, header := range headers {
		cell, err := excelize.CoordinatesToCellName(index+1, 1)
		if err != nil {
			t.Fatalf("header cell: %v", err)
		}
		if err := file.SetCellValue("Sheet1", cell, header); err != nil {
			t.Fatalf("header value: %v", err)
		}
	}
	for rowNumber, row := range dataRows {
		for index, value := range row {
			cell, err := excelize.CoordinatesToCellName(index+1, rowNumber+2)
			if err != nil {
				t.Fatalf("cell: %v", err)
			}
			if err := file.SetCellValue("Sheet1", cell, value); err != nil {
				t.Fatalf("cell value: %v", err)
			}
		}
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("write workbook: %v", err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "teachers.xlsx")
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := part.Write(buffer.Bytes()); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.WriteField("mode", "create_only"); err != nil {
		t.Fatalf("mode field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/imports/teachers", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result teacherUploadResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode upload: %v", err)
	}
	return result
}

func sheetHeader(t *testing.T, workbook []byte) []string {
	t.Helper()
	file, err := excelize.OpenReader(bytes.NewReader(workbook))
	if err != nil {
		t.Fatalf("open workbook: %v", err)
	}
	defer func() { _ = file.Close() }()
	sheets := file.GetSheetList()
	if len(sheets) == 0 {
		t.Fatal("workbook has no sheets")
	}
	rows, err := file.GetRows(sheets[0])
	if err != nil || len(rows) == 0 {
		t.Fatalf("workbook header rows: %v", err)
	}
	return rows[0]
}

func pgNilUUID() string { return "00000000-0000-0000-0000-000000000000" }

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
	name := fmt.Sprintf("ifter_employmenttype_test_%d", time.Now().UnixNano())
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
	importService, err := importservice.NewService(pool, log, importservice.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("build import service: %v", err)
	}
	// The pgx pool doubles as readiness; New() wires the employment-type
	// service from it (see router.go FeatureDeps). Master data, imports and
	// reporting are exercised together because the package contract spans
	// them (teacher DTO, Jenis Kepegawaian column, dashboard breakdown).
	router := httptransport.New(log, pool, authService, masterdata.NewService(pool, log),
		importService, nil, nil, reporting.NewService(pool, log), config.AuthConfig{
			LoginRateLimit:  1000,
			LoginRateWindow: time.Minute,
		}, nil)
	return apiEnv{pool: pool, router: router}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	e.insertUser(t, "emp-admin", "emp-admin@example.com", "emp-admin-pw", "admin")
	e.insertUser(t, "emp-staff", "emp-staff@example.com", "emp-staff-pw", "staff_tu")
	e.insertUser(t, "emp-teacher", "emp-teacher@example.com", "emp-teacher-pw", "teacher")
	e.insertUser(t, "emp-siswa", "emp-siswa@example.com", "emp-siswa-pw", "student")
	return testData{
		adminToken:   e.login(t, "emp-admin", "emp-admin-pw"),
		staffToken:   e.login(t, "emp-staff", "emp-staff-pw"),
		teacherToken: e.login(t, "emp-teacher", "emp-teacher-pw"),
		studentToken: e.login(t, "emp-siswa", "emp-siswa-pw"),
		permanentID:  e.seededTypeID(t, "permanent"),
		honoraryID:   e.seededTypeID(t, "honorary"),
		staffTuID:    e.seededTypeID(t, "staff_tu"),
	}
}

func (e apiEnv) seededTypeID(t *testing.T, code string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		"SELECT id::text FROM employment_types WHERE code = $1", code,
	).Scan(&id); err != nil {
		t.Fatalf("seeded employment type %q missing: %v", code, err)
	}
	return id
}

func (e apiEnv) insertUser(t *testing.T, username, email, password string, roles ...string) {
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
		t.Fatalf("insert user: %v", err)
	}
	for _, role := range roles {
		if _, err := e.pool.Exec(context.Background(),
			"INSERT INTO user_roles (user_id, role_code) VALUES ($1,$2)", id, role,
		); err != nil {
			t.Fatalf("assign role %q: %v", role, err)
		}
	}
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
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode login: %v", err)
	}
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
