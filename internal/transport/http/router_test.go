package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
	"github.com/ghalitsar-coder/backend-ifter/internal/qrattendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/recap"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeReadiness struct {
	err error
}

func (f fakeReadiness) Ping(context.Context) error {
	return f.err
}

func testDeps() Deps {
	return Deps{
		Config: &config.Config{
			App: config.AppConfig{Name: "test", Env: "test"},
			HTTP: config.HTTPConfig{
				MaxBodyBytes:      1 << 20,
				RateLimitRequests: 100,
				RateLimitWindow:   time.Minute,
			},
		},
		Logger:    testLogger(),
		Metrics:   metrics.New(),
		Readiness: fakeReadiness{},
		Version:   "test",
	}
}

func TestRouterStatusEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(testDeps())

	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/readyz", http.StatusOK},
		{http.MethodGet, "/", http.StatusOK},
		{http.MethodGet, "/v1/version", http.StatusOK},
		{http.MethodGet, "/metrics", http.StatusOK},
		{http.MethodGet, "/missing", http.StatusNotFound},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestRouterMetricsExposed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(testDeps())

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Error("metrics body does not contain go_goroutines")
	}
}

func TestRouterReadinessFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps := testDeps()
	deps.Readiness = fakeReadiness{err: errors.New("database down")}

	rec := httptest.NewRecorder()
	New(deps).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestRouterRequestIDAndErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(testDeps())

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("X-Request-ID", "req-123")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "req-123" {
		t.Errorf("X-Request-ID = %q, want req-123", got)
	}

	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Errorf("error.code = %q, want not_found", body.Error.Code)
	}
	if body.Error.RequestID != "req-123" {
		t.Errorf("error.request_id = %q, want req-123", body.Error.RequestID)
	}
}

// fakeLoginRepository serves one stored user so the login route is tested
// without a database.
type fakeLoginRepository struct{ user auth.User }

func (f fakeLoginRepository) FindByUsername(context.Context, string) (auth.User, error) {
	return f.user, nil
}

// classesRepository satisfies attendance.Repository with only the read this
// test needs. Any other method would panic, which is fine: no other route is
// called.
type classesRepository struct{ attendance.Repository }

func (classesRepository) ListClasses(context.Context, int64) ([]attendance.Class, error) {
	return []attendance.Class{{ID: 1, NamaKelas: "1A", Tingkat: 1}}, nil
}

type fakeRegisterRepository struct {
	created auth.RegisterResult
	err     error
}

func (f fakeRegisterRepository) RegisterSchoolAdmin(context.Context, auth.RegisterInput, string, string) (auth.RegisterResult, error) {
	return f.created, f.err
}

func testAuthDeps(t *testing.T) Deps {
	t.Helper()
	gin.SetMode(gin.TestMode)

	deps := testDeps()
	hash, err := bcrypt.GenerateFromPassword([]byte("guru123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate hash: %v", err)
	}

	tokens := auth.NewTokenService(config.AuthConfig{
		JWTSecret:      strings.Repeat("k", 32),
		JWTIssuer:      "test-issuer",
		JWTAudience:    "test-audience",
		AccessTokenTTL: time.Hour,
	})
	deps.Tokens = tokens
	deps.Auth = auth.NewLoginService(
		fakeLoginRepository{user: auth.User{
			ID:           1,
			SchoolID:     1,
			Username:     "guru1",
			PasswordHash: string(hash),
			IsActive:     true,
			Nama:         "Ibu Sari",
			Roles:        []string{"guru"},
		}},
		tokens,
		time.Hour,
	)
	deps.Attendance = attendance.NewService(classesRepository{})
	deps.Register = auth.NewRegisterService(fakeRegisterRepository{
		created: auth.RegisterResult{UserID: 7, SchoolID: 2, Username: "admin@sekolah.sch.id"},
	}, tokens, time.Hour)
	return deps
}

func TestRouterLoginIsPublicAndClassesRequireBearer(t *testing.T) {
	router := New(testAuthDeps(t))

	var login struct {
		Data struct {
			Token string   `json:"token"`
			Nama  string   `json:"nama"`
			Roles []string `json:"roles"`
		} `json:"data"`
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/auth/login", `{"username":"guru1","password":"guru123"}`, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login body: %v", err)
	}
	if login.Data.Token == "" || login.Data.Nama != "Ibu Sari" || len(login.Data.Roles) != 1 {
		t.Fatalf("login data = %+v, want a token, name and one role", login.Data)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodGet, "/v1/classes", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("classes without a bearer token = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodGet, "/v1/classes", "", "Bearer "+login.Data.Token))
	if rec.Code != http.StatusOK {
		t.Fatalf("classes with the issued token = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"nama_kelas":"1A"`) {
		t.Errorf("classes body = %s, want the seeded class", rec.Body.String())
	}
}

func TestRouterLoginRejectsBadPassword(t *testing.T) {
	router := New(testAuthDeps(t))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/auth/login", `{"username":"guru1","password":"salah"}`, ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error.Code != "invalid_credentials" {
		t.Errorf("error.code = %q, want invalid_credentials", body.Error.Code)
	}
}

type routeRecapRepository struct{}

func (routeRecapRepository) GenerateStudentRecaps(context.Context, int64, int16, int16) error {
	return nil
}

func (routeRecapRepository) ListStudentRecaps(context.Context, int64, int16, int16) ([]recap.StudentRecap, error) {
	return []recap.StudentRecap{}, nil
}

func (routeRecapRepository) StudentRecapStatus(context.Context, int64, int64) (string, error) {
	return "draft", nil
}

func (routeRecapRepository) VerifyStudentRecap(context.Context, int64, int64, int64) (recap.Verification, error) {
	return recap.Verification{}, nil
}

func (routeRecapRepository) GenerateStaffRecaps(context.Context, int64, int16, int16) error {
	return nil
}

func (routeRecapRepository) ListStaffRecaps(context.Context, int64, int16, int16) ([]recap.StaffRecap, error) {
	return []recap.StaffRecap{}, nil
}

func (routeRecapRepository) StaffRecapStatus(context.Context, int64, int64) (string, error) {
	return "draft", nil
}

func (routeRecapRepository) VerifyStaffRecap(context.Context, int64, int64, int64) (recap.Verification, error) {
	return recap.Verification{}, nil
}

func TestStudentRecapRoutesRequireTUOrAdmin(t *testing.T) {
	deps := testAuthDeps(t)
	deps.Recaps = recap.NewService(routeRecapRepository{})
	router := New(deps)

	var login struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/auth/login", `{"username":"guru1","password":"guru123"}`, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/recaps/students", `{"bulan":10,"tahun":2026}`, "Bearer "+login.Data.Token))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("guru recap status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodGet, "/v1/recaps/students?bulan=10&tahun=2026", "", ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous recap status = %d, want %d: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestStaffRecapRoutesRequireTUOrAdmin(t *testing.T) {
	deps := testAuthDeps(t)
	deps.Recaps = recap.NewService(routeRecapRepository{})
	router := New(deps)

	var login struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/auth/login", `{"username":"guru1","password":"guru123"}`, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/recaps/staff", `{"bulan":10,"tahun":2026}`, "Bearer "+login.Data.Token))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("guru recap status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

type routeQRRepository struct{}

func (routeQRRepository) CreateSession(_ context.Context, schoolID, creatorID int64, secret string, input qrattendance.CreateSessionInput, date time.Time, openAt, closeAt string, lateAfter *string) (qrattendance.SessionRecord, error) {
	return qrattendance.SessionRecord{
		Session:  qrattendance.Session{ID: 1, Date: date.Format("2006-01-02"), Type: input.Type, OpenAt: openAt, CloseAt: closeAt, LateAfter: lateAfter, RotationSeconds: 30, Status: "aktif"},
		SchoolID: schoolID, CreatedBy: creatorID, TokenSecret: secret,
	}, nil
}

func (routeQRRepository) GetSession(context.Context, int64, int64) (qrattendance.SessionRecord, error) {
	return qrattendance.SessionRecord{}, nil
}

func (routeQRRepository) CloseSession(context.Context, int64, int64) error             { return nil }
func (routeQRRepository) FindActiveStaff(context.Context, int64, int64) (int64, error) { return 1, nil }
func (routeQRRepository) CheckIn(context.Context, int64, int64, time.Time, time.Time, string) error {
	return nil
}
func (routeQRRepository) CheckOut(context.Context, int64, int64, time.Time, time.Time) error {
	return nil
}

func TestQRSessionCreationRequiresTUOrAdmin(t *testing.T) {
	deps := testAuthDeps(t)
	deps.QRAttendance = qrattendance.NewService(routeQRRepository{})
	secret := strings.Repeat("q", 32)
	tokens := auth.NewTokenService(config.AuthConfig{JWTSecret: secret, JWTIssuer: "test-issuer", JWTAudience: "test-audience", AccessTokenTTL: time.Hour})
	deps.Tokens = tokens
	teacherToken, err := tokens.Issue("1", []string{"guru"}, 1)
	if err != nil {
		t.Fatalf("issue teacher token: %v", err)
	}
	tuToken, err := tokens.Issue("2", []string{"tu"}, 1)
	if err != nil {
		t.Fatalf("issue TU token: %v", err)
	}
	adminToken, err := tokens.Issue("3", []string{"admin"}, 1)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	router := New(deps)
	body := `{"tanggal":"2097-11-16","tipe_sesi":"masuk","jam_buka":"07:00","jam_tutup":"09:00"}`

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/staff/qr-sessions", body, "Bearer "+teacherToken))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("guru create QR status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/staff/qr-sessions", body, "Bearer "+adminToken))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("admin create QR status = %d, want %d: %s", rec.Code, http.StatusForbidden, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/staff/qr-sessions", body, "Bearer "+tuToken))
	if rec.Code != http.StatusCreated {
		t.Fatalf("TU create QR status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "token_secret") || strings.Contains(rec.Body.String(), "TokenSecret") {
		t.Fatalf("QR session response leaked its secret: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/staff/attendance/scan", `{"payload":"bad"}`, ""))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous scan status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

type routeDisciplineRepository struct{}

func (routeDisciplineRepository) StaffIDByUserID(context.Context, int64) (int64, error) {
	return 1, nil
}
func (routeDisciplineRepository) ClassInSchool(context.Context, int64, int64) (bool, error) {
	return true, nil
}
func (routeDisciplineRepository) ListViolationTypes(context.Context, int64) ([]discipline.ViolationType, error) {
	return []discipline.ViolationType{{ID: 1, SchoolID: 1, Kode: "ATR-01", Nama: "Seragam", PoinDefault: 3}}, nil
}
func (routeDisciplineRepository) GetViolationTypeByID(context.Context, int64, int64) (discipline.ViolationType, error) {
	return discipline.ViolationType{ID: 1, SchoolID: 1, Kode: "ATR-01", Nama: "Seragam", PoinDefault: 3}, nil
}
func (routeDisciplineRepository) RecordViolation(_ context.Context, schoolID int64, in discipline.RecordViolationInput, poin int32, _ time.Time) (discipline.StudentViolation, error) {
	return discipline.StudentViolation{ID: 1, SchoolID: schoolID, StudentID: in.StudentID, Poin: poin}, nil
}
func (routeDisciplineRepository) ListViolations(context.Context, int64, discipline.ListViolationsFilter) ([]discipline.StudentViolation, error) {
	return []discipline.StudentViolation{}, nil
}
func (routeDisciplineRepository) GetStudentSummary(_ context.Context, _ int64, studentID int64) (discipline.StudentSummary, error) {
	return discipline.StudentSummary{StudentID: studentID}, nil
}
func (routeDisciplineRepository) UpdateFollowup(_ context.Context, schoolID, id int64, followup string) (discipline.StudentViolation, error) {
	return discipline.StudentViolation{ID: id, SchoolID: schoolID, TindakLanjut: &followup}, nil
}
func (routeDisciplineRepository) CreateInspectionWithViolations(_ context.Context, schoolID, inspectorID int64, in discipline.CreateInspectionInput, _ time.Time, _ map[int64]int32) (discipline.DisciplineInspection, error) {
	return discipline.DisciplineInspection{ID: 1, SchoolID: schoolID, ClassID: in.ClassID, InspectorID: inspectorID}, nil
}
func (routeDisciplineRepository) ListInspections(context.Context, int64, discipline.ListInspectionsFilter) ([]discipline.DisciplineInspection, error) {
	return []discipline.DisciplineInspection{}, nil
}
func (routeDisciplineRepository) ListClassStudents(context.Context, int64, int64) ([]discipline.ClassStudent, error) {
	return []discipline.ClassStudent{}, nil
}

func TestDisciplineRoutesRequireKesiswaan(t *testing.T) {
	deps := testAuthDeps(t)
	deps.Discipline = discipline.NewService(routeDisciplineRepository{})
	secret := strings.Repeat("d", 32)
	tokens := auth.NewTokenService(config.AuthConfig{JWTSecret: secret, JWTIssuer: "test-issuer", JWTAudience: "test-audience", AccessTokenTTL: time.Hour})
	deps.Tokens = tokens

	guruToken, err := tokens.Issue("1", []string{"guru"}, 1)
	if err != nil {
		t.Fatalf("issue guru token: %v", err)
	}
	adminToken, err := tokens.Issue("2", []string{"admin"}, 1)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}
	tuToken, err := tokens.Issue("3", []string{"tu"}, 1)
	if err != nil {
		t.Fatalf("issue tu token: %v", err)
	}
	kepsekToken, err := tokens.Issue("4", []string{"kepala_sekolah"}, 1)
	if err != nil {
		t.Fatalf("issue kepsek token: %v", err)
	}
	kesiswaanToken, err := tokens.Issue("5", []string{"kesiswaan"}, 1)
	if err != nil {
		t.Fatalf("issue kesiswaan token: %v", err)
	}

	router := New(deps)
	recordBody := `{"student_id":1,"violation_type_id":1,"tanggal":"2026-11-22"}`
	inspectionBody := `{"class_id":1,"tanggal":"2026-11-22","jumlah_diperiksa":25,"violations":[]}`
	followupBody := `{"tindak_lanjut":"Pembinaan"}`

	denied := []struct {
		name  string
		token string
	}{
		{"guru", guruToken},
		{"admin", adminToken},
		{"tu", tuToken},
		{"kepala_sekolah", kepsekToken},
	}

	for _, actor := range denied {
		endpoints := []struct {
			method string
			path   string
			body   string
		}{
			{http.MethodGet, "/v1/discipline/types", ""},
			{http.MethodGet, "/v1/discipline/records", ""},
			{http.MethodPost, "/v1/discipline/records", recordBody},
			{http.MethodPatch, "/v1/discipline/records/1/followup", followupBody},
			{http.MethodGet, "/v1/discipline/inspections", ""},
			{http.MethodPost, "/v1/discipline/inspections", inspectionBody},
			{http.MethodGet, "/v1/discipline/students/1/summary", ""},
			{http.MethodGet, "/v1/discipline/classes/1/students", ""},
		}
		for _, ep := range endpoints {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, jsonRequest(ep.method, ep.path, ep.body, "Bearer "+actor.token))
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s %s = %d, want %d: %s", actor.name, ep.method, ep.path, rec.Code, http.StatusForbidden, rec.Body.String())
			}
		}
	}

	allowed := []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodGet, "/v1/discipline/types", "", http.StatusOK},
		{http.MethodGet, "/v1/discipline/records", "", http.StatusOK},
		{http.MethodPost, "/v1/discipline/records", recordBody, http.StatusCreated},
		{http.MethodPatch, "/v1/discipline/records/1/followup", followupBody, http.StatusOK},
		{http.MethodGet, "/v1/discipline/inspections", "", http.StatusOK},
		{http.MethodPost, "/v1/discipline/inspections", inspectionBody, http.StatusCreated},
		{http.MethodGet, "/v1/discipline/students/1/summary", "", http.StatusOK},
		{http.MethodGet, "/v1/discipline/classes/1/students", "", http.StatusOK},
	}
	for _, ep := range allowed {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, jsonRequest(ep.method, ep.path, ep.body, "Bearer "+kesiswaanToken))
		if rec.Code != ep.want {
			t.Errorf("kesiswaan %s %s = %d, want %d: %s", ep.method, ep.path, rec.Code, ep.want, rec.Body.String())
		}
	}
}

func TestRouterRegisterCreatesSchoolAndReturnsAdminToken(t *testing.T) {
	router := New(testAuthDeps(t))

	body := `{"school_name":"SMP Negeri 1","npsn":"12345678","jenjang":"smp","admin_name":"Budi","email":"Admin@Sekolah.sch.id","phone":"081234567890","password":"rahasia123"}`
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/auth/register", body, ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var result struct {
		Data struct {
			Token    string   `json:"token"`
			Username string   `json:"username"`
			Nama     string   `json:"nama"`
			Roles    []string `json:"roles"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode register body: %v", err)
	}
	if result.Data.Token == "" || result.Data.Username != "admin@sekolah.sch.id" ||
		result.Data.Nama != "Budi" || len(result.Data.Roles) != 1 || result.Data.Roles[0] != "admin" {
		t.Fatalf("register data = %+v, want an admin token for the normalized email", result.Data)
	}
}

func TestRouterRegisterRejectsInvalidPayload(t *testing.T) {
	router := New(testAuthDeps(t))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, jsonRequest(http.MethodPost, "/v1/auth/register", `{"school_name":"SMP Negeri 1"}`, ""))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
	}
}

func jsonRequest(method, path, body, authorization string) *http.Request {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	return req
}
