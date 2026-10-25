package correctionevidence_test

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
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	attendancepkg "github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/correctionevidence"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

const (
	attendanceDay = "2026-09-02" // Wednesday, inside the seeded Mon-Fri schedule
	correctionPNG = "evidence.png"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
	dir    string
}

type testData struct {
	adminID       string
	adminToken    string
	homeroomToken string
	otherTeach    string
	dutyToken     string
	principalTok  string
	classA        string
	recordID      string
}

func TestCorrectionEvidenceAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	// Close the day and correct through the existing attendance endpoints;
	// the correction response must carry the server-computed evidence hash.
	if rec := env.request(t, http.MethodPost, "/api/v1/attendance/close", map[string]string{
		"class_id": data.classA, "date": attendanceDay,
	}, data.adminToken); rec.Code != http.StatusOK {
		t.Fatalf("close status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec := env.request(t, http.MethodPost, "/api/v1/attendance/records/"+data.recordID+"/corrections", map[string]any{
		"status": "late", "check_in_at": "07:10", "notes": "dikoreksi", "reason": "salah input",
	}, data.adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("correction status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var detail attendancepkg.RecordDetail
	decode(t, rec, &detail)
	if len(detail.Corrections) != 1 {
		t.Fatalf("corrections = %d, want 1", len(detail.Corrections))
	}
	first := detail.Corrections[0]
	if first.EvidenceHash == nil || !hex64.MatchString(*first.EvidenceHash) {
		t.Fatalf("correction evidence hash = %v, want 64-char hex sha256", first.EvidenceHash)
	}
	// The hash must equal sha256 over the exact canonical payload.
	if first.NewCheckInAt == nil {
		t.Fatal("corrected check_in_at missing")
	}
	firstHash := *first.EvidenceHash
	want := correctionevidence.HashCorrectionFields(
		first.OldStatus, first.NewStatus,
		pgtype.Timestamptz{Time: *first.NewCheckInAt, Valid: true},
		pgtype.Text{String: "dikoreksi", Valid: true},
		"salah input", data.adminID, first.CorrectedAt,
	)
	if want != *first.EvidenceHash {
		t.Fatalf("evidence hash = %s, recomputed = %s", *first.EvidenceHash, want)
	}

	// A second correction gets its own distinct hash.
	rec = env.request(t, http.MethodPost, "/api/v1/attendance/records/"+data.recordID+"/corrections", map[string]any{
		"notes": "final", "reason": "revisi kedua",
	}, data.adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("second correction status = %d, body = %s", rec.Code, rec.Body.String())
	}
	// Decode into a fresh struct: encoding/json reuses the *string pointer
	// fields of the previously decoded detail, which would alias (and
	// overwrite) first.EvidenceHash.
	var detail2 attendancepkg.RecordDetail
	decode(t, rec, &detail2)
	if len(detail2.Corrections) != 2 {
		t.Fatalf("corrections = %d, want 2", len(detail2.Corrections))
	}
	second := detail2.Corrections[0] // list is corrected_at DESC
	if second.EvidenceHash == nil || *second.EvidenceHash == firstHash {
		t.Fatalf("second correction hash missing or equal to first: second=%s first=%s",
			jsonString(t, second), jsonString(t, first))
	}
	// The second hash must cover the second correction's own payload.
	wantSecond := correctionevidence.HashCorrectionFields(
		second.OldStatus, second.NewStatus,
		pgtype.Timestamptz{Time: *second.NewCheckInAt, Valid: true},
		pgtype.Text{String: "final", Valid: true},
		"revisi kedua", data.adminID, second.CorrectedAt,
	)
	if wantSecond != *second.EvidenceHash {
		t.Fatalf("second evidence hash = %s, recomputed = %s", *second.EvidenceHash, wantSecond)
	}

	// GET /attendance/records/:id exposes the same hash.
	rec = env.request(t, http.MethodGet, "/api/v1/attendance/records/"+data.recordID, nil, data.adminToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("record detail status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var fetched attendancepkg.RecordDetail
	decode(t, rec, &fetched)
	for _, correction := range fetched.Corrections {
		if correction.EvidenceHash == nil || !hex64.MatchString(*correction.EvidenceHash) {
			t.Fatalf("record detail correction %s has no evidence hash", correction.ID)
		}
	}

	t.Run("upload requires a valid document", func(t *testing.T) {
		if rec := env.uploadEvidence(t, data.adminToken, data.recordID, "notes.txt",
			"text/plain", []byte("bukan dokumen"), "", ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad extension status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.uploadEvidence(t, data.adminToken, data.recordID, "fake.png",
			"image/png", []byte("definitely not a png"), "", ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad magic bytes status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.uploadEvidence(t, data.adminToken, data.recordID, correctionPNG,
			"image/png", pngBytes(2048), "", ""); rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("oversize status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.uploadEvidence(t, data.adminToken, data.recordID, correctionPNG,
			"image/png", pngBytes(64), "", "22222222-2222-2222-2222-222222222222"); rec.Code != http.StatusBadRequest {
			t.Fatalf("foreign correction id status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("role and scope gates", func(t *testing.T) {
		if rec := env.uploadEvidence(t, data.principalTok, data.recordID, correctionPNG,
			"image/png", pngBytes(64), "", ""); rec.Code != http.StatusForbidden {
			t.Fatalf("principal upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.uploadEvidence(t, data.otherTeach, data.recordID, correctionPNG,
			"image/png", pngBytes(64), "", ""); rec.Code != http.StatusForbidden {
			t.Fatalf("non-homeroom teacher upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	// Admin upload: links automatically to the newest correction.
	rec = env.uploadEvidence(t, data.adminToken, data.recordID, correctionPNG,
		"image/png", pngBytes(64), "bukti fotokopi", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("admin upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var uploadBody struct {
		Evidence correctionevidence.EvidenceItem `json:"evidence"`
	}
	decode(t, rec, &uploadBody)
	admin := uploadBody.Evidence
	if admin.AttendanceRecordID != data.recordID || admin.FileName != correctionPNG ||
		admin.ContentType != "image/png" || admin.SizeBytes != 64 ||
		admin.UploadedBy != data.adminID {
		t.Fatalf("admin evidence = %+v", admin)
	}
	if admin.CorrectionID == nil || *admin.CorrectionID != second.ID {
		t.Fatalf("evidence correction link = %v, want newest %s", admin.CorrectionID, second.ID)
	}
	if admin.Note == nil || *admin.Note != "bukti fotokopi" {
		t.Fatalf("evidence note = %v", admin.Note)
	}
	if _, err := os.Stat(filepath.Join(env.dir, filepath.Base(*admin.CorrectionID))); err == nil {
		t.Fatal("stored key must be opaque, not derived from ids")
	}

	// Duty teacher (on duty Wednesday) may attach evidence school-wide.
	rec = env.uploadEvidence(t, data.dutyToken, data.recordID, "bukti-duty.png",
		"image/png", pngBytes(32), "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("duty upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var dutyBody struct {
		Evidence correctionevidence.EvidenceItem `json:"evidence"`
	}
	decode(t, rec, &dutyBody)
	duty := dutyBody.Evidence

	t.Run("list", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/attendance/records/"+data.recordID+"/corrections/evidence", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Data []correctionevidence.EvidenceItem `json:"data"`
		}
		decode(t, rec, &body)
		if len(body.Data) != 2 || body.Data[0].ID != duty.ID || body.Data[1].ID != admin.ID {
			t.Fatalf("list = %+v, want newest-first [%s, %s]", body.Data, duty.ID, admin.ID)
		}
		if raw := rec.Body.String(); strings.Contains(raw, "stored_key") {
			t.Fatalf("list leaked storage internals: %s", raw)
		}
		// Non-homeroom teacher cannot even list.
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance/records/"+data.recordID+"/corrections/evidence", nil, data.otherTeach); rec.Code != http.StatusForbidden {
			t.Fatalf("scoped list status = %d, want 403", rec.Code)
		}
	})

	t.Run("download", func(t *testing.T) {
		rec := env.rawRequest(http.MethodGet, "/api/v1/attendance/corrections/evidence/"+admin.ID+"/download", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("download status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, `attachment; filename="`+correctionPNG+`"`) {
			t.Fatalf("content disposition = %q", got)
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("nosniff header = %q", got)
		}
		if !bytes.HasPrefix(rec.Body.Bytes(), []byte{0x89, 'P', 'N', 'G'}) {
			t.Fatalf("downloaded bytes are not the uploaded png: %d bytes", rec.Body.Len())
		}
		if rec := env.rawRequest(http.MethodGet, "/api/v1/attendance/corrections/evidence/"+admin.ID+"/download", nil, data.otherTeach); rec.Code != http.StatusForbidden {
			t.Fatalf("scoped download status = %d, want 403", rec.Code)
		}
	})

	t.Run("delete", func(t *testing.T) {
		rec := env.rawRequest(http.MethodDelete, "/api/v1/attendance/corrections/evidence/"+duty.ID, nil, data.adminToken)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.rawRequest(http.MethodDelete, "/api/v1/attendance/corrections/evidence/"+duty.ID, nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("re-delete status = %d, want 404", rec.Code)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/attendance/records/"+data.recordID+"/corrections/evidence", nil, data.adminToken)
		var body struct {
			Data []correctionevidence.EvidenceItem `json:"data"`
		}
		decode(t, rec, &body)
		if len(body.Data) != 1 || body.Data[0].ID != admin.ID {
			t.Fatalf("list after delete = %+v", body.Data)
		}
	})

	// Sensitive writes are audited.
	for action, want := range map[string]int{
		"correction_evidence.add":    2,
		"correction_evidence.delete": 1,
	} {
		var count int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM audit_logs WHERE action = $1", action,
		).Scan(&count); err != nil {
			t.Fatalf("count audit %s: %v", action, err)
		}
		if count != want {
			t.Fatalf("audit rows for %q = %d, want %d", action, count, want)
		}
	}

	t.Run("unknown record", func(t *testing.T) {
		rec := env.uploadEvidence(t, data.adminToken, "33333333-3333-3333-3333-333333333333",
			correctionPNG, "image/png", pngBytes(16), "", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("unknown record upload status = %d, want 404", rec.Code)
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
	name := fmt.Sprintf("ifter_correction_evidence_test_%d", time.Now().UnixNano())
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
	dir := t.TempDir()
	// router.New builds the correction-evidence service on the pool passed as
	// readiness, configured from the environment (ConfigFromEnvironment), so
	// point the storage dir at the temp dir and keep the 1 KiB cap the tests
	// upload against.
	t.Setenv("IFTER_IMPORTS_DIR", dir)
	t.Setenv("IFTER_IMPORTS_MAX_FILE_SIZE", "1024")

	tokens := authpkg.NewTokenManager("test-secret-at-least-32-characters-long", "test-issuer", 15*time.Minute)
	authService := authpkg.NewService(pool, tokens, authpkg.ServiceConfig{
		RefreshTokenTTL: 24 * time.Hour,
		InviteTokenTTL:  72 * time.Hour,
		ResetTokenTTL:   time.Hour,
		BcryptCost:      bcrypt.MinCost,
	}, log)
	attendanceService := attendancepkg.NewService(pool, log)
	router := httptransport.New(log, pool, authService, nil, nil, attendanceService, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router, dir: dir}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	adminID := e.insertUser(t, "ce-admin", "ce-admin@example.com", "ce-admin-password", "admin")
	hrUserID := e.insertUser(t, "ce-hr", "ce-hr@example.com", "ce-hr-password", "teacher")
	otherUserID := e.insertUser(t, "ce-other", "ce-other@example.com", "ce-other-password", "teacher")
	dutyID := e.insertUser(t, "ce-duty", "ce-duty@example.com", "ce-duty-password", "duty_teacher")
	principalID := e.insertUser(t, "ce-principal", "ce-principal@example.com", "ce-principal-password", "principal")
	_ = principalID

	yearID := e.insert(t,
		`INSERT INTO academic_years (name, start_date, end_date, is_active)
		 VALUES ('2026/2027', '2026-07-01', '2027-06-30', true) RETURNING id::text`)
	classA := e.insert(t, `INSERT INTO classes (academic_year_id, name, grade_level) VALUES ($1, '7A', 7) RETURNING id::text`, yearID)
	classB := e.insert(t, `INSERT INTO classes (academic_year_id, name, grade_level) VALUES ($1, '7B', 7) RETURNING id::text`, yearID)

	teacherA := e.insertTeacher(t, hrUserID, "Guru Wali A")
	teacherB := e.insertTeacher(t, otherUserID, "Guru Wali B")
	teacherDuty := e.insertTeacher(t, dutyID, "Guru Piket")
	e.insertHomeroom(t, teacherA, classA)
	e.insertHomeroom(t, teacherB, classB)
	e.insertDuty(t, teacherDuty, 3) // Wednesday

	studentA := e.insertStudent(t, "CE-NIS-A", "Siswa A")
	e.insertEnrollment(t, studentA, classA)
	recordID := e.insert(t,
		`INSERT INTO attendance_records (student_id, enrollment_id, attendance_date, status, source, recorded_by_user_id, updated_by_user_id)
		 VALUES ($1, (SELECT id FROM class_enrollments WHERE student_id = $1), $2, 'present', 'manual', $3, $3) RETURNING id::text`,
		studentA, attendanceDay, adminID)

	return testData{
		adminID:       adminID,
		adminToken:    e.login(t, "ce-admin", "ce-admin-password"),
		homeroomToken: e.login(t, "ce-hr", "ce-hr-password"),
		otherTeach:    e.login(t, "ce-other", "ce-other-password"),
		dutyToken:     e.login(t, "ce-duty", "ce-duty-password"),
		principalTok:  e.login(t, "ce-principal", "ce-principal-password"),
		classA:        classA,
		recordID:      recordID,
	}
}

func (e apiEnv) insert(t *testing.T, query string, args ...any) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&id); err != nil {
		t.Fatalf("insert (%s): %v", query, err)
	}
	return id
}

func (e apiEnv) insertUser(t *testing.T, username, email, password string, roles ...string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	id := e.insert(t,
		`INSERT INTO users (username, email, password_hash, full_name, status)
		 VALUES ($1, $2, $3, $4, 'active') RETURNING id::text`,
		username, email, string(hash), "Test "+username)
	for _, role := range roles {
		if _, err := e.pool.Exec(context.Background(),
			"INSERT INTO user_roles (user_id, role_code) VALUES ($1,$2)", id, role,
		); err != nil {
			t.Fatalf("assign role %q: %v", role, err)
		}
	}
	return id
}

func (e apiEnv) insertTeacher(t *testing.T, userID, name string) string {
	t.Helper()
	return e.insert(t, `INSERT INTO teachers (user_id, full_name) VALUES ($1,$2) RETURNING id::text`, userID, name)
}

func (e apiEnv) insertStudent(t *testing.T, nis, name string) string {
	t.Helper()
	return e.insert(t,
		`INSERT INTO students (student_number, full_name, admission_year) VALUES ($1, $2, 2026) RETURNING id::text`,
		nis, name)
}

func (e apiEnv) insertEnrollment(t *testing.T, studentID, classID string) {
	t.Helper()
	e.insert(t,
		`INSERT INTO class_enrollments (student_id, class_id, start_date, end_date)
		 VALUES ($1, $2, '2026-09-01', '2027-06-30') RETURNING id::text`,
		studentID, classID)
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
	rec := e.rawRequest(method, path, reader, token, func(req *http.Request) {
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
	})
	return rec
}

func (e apiEnv) rawRequest(method, path string, body io.Reader, token string, opts ...func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for _, opt := range opts {
		opt(req)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e apiEnv) uploadEvidence(t *testing.T, token, recordID, filename, contentType string,
	data []byte, note, correctionID string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	if contentType != "" {
		partHeader.Set("Content-Type", contentType)
	}
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(data)); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if note != "" {
		if err := writer.WriteField("note", note); err != nil {
			t.Fatalf("write note field: %v", err)
		}
	}
	if correctionID != "" {
		if err := writer.WriteField("correction_id", correctionID); err != nil {
			t.Fatalf("write correction_id field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}
	return e.rawRequest(http.MethodPost, "/api/v1/attendance/records/"+recordID+"/corrections/evidence",
		&buf, token, func(req *http.Request) {
			req.Header.Set("Content-Type", writer.FormDataContentType())
		})
}

func pngBytes(fill int) []byte {
	data := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	for len(data) < fill {
		data = append(data, 'x')
	}
	return data
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func jsonString(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%+v", value)
	}
	return string(data)
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
