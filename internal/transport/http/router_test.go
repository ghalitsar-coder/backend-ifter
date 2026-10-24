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
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
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
