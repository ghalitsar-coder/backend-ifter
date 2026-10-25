package teacherpresence_test

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
	teacherpresencepkg "github.com/ghalitsar-coder/backend-ifter/internal/teacherpresence"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type testData struct {
	adminToken     string
	teacherID      string
	teacherToken   string
	dutyToken      string
	loneToken      string
	principalToken string
}

// schoolCenter mirrors the 000009 seed row (central Jakarta, 300 m radius).
const (
	schoolLat  = -6.200000
	schoolLng  = 106.816666
	insideLat  = -6.200500 // ~55.6 m north of the seed center
	outsideLat = -6.300000 // ~11.1 km north of the seed center
)

func TestTeacherPresenceAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	checkIn := func(t *testing.T, token string, body map[string]any) *httptest.ResponseRecorder {
		return env.request(t, http.MethodPost, "/api/v1/teacher-presence/check-in", body, token)
	}

	t.Run("rbac and scope", func(t *testing.T) {
		if rec := checkIn(t, "", map[string]any{"latitude": insideLat, "longitude": schoolLng, "accuracy": 10}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous check-in status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := checkIn(t, data.principalToken, map[string]any{"latitude": insideLat, "longitude": schoolLng, "accuracy": 10}); rec.Code != http.StatusForbidden {
			t.Fatalf("principal check-in status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// Teacher role without a linked teachers profile: use case must 403.
		if rec := checkIn(t, data.loneToken, map[string]any{"latitude": insideLat, "longitude": schoolLng, "accuracy": 10}); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher-without-profile check-in status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/teacher-presence/today", nil, data.loneToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher-without-profile today status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("geofence config", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/geofence", nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("teacher get geofence status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var seeded teacherpresencepkg.GeofenceConfig
		decode(t, rec, &seeded)
		if seeded.RadiusMeters != 300 || !seeded.IsEnabled {
			t.Fatalf("seeded geofence = %+v", seeded)
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/geofence", map[string]any{
			"latitude": schoolLat, "longitude": schoolLng, "radius_meters": 300, "is_enabled": true,
		}, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher put geofence status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/geofence", map[string]any{
			"latitude": 100, "longitude": schoolLng, "radius_meters": 300, "is_enabled": true,
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid latitude put geofence status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/geofence", map[string]any{
			"latitude": schoolLat, "longitude": schoolLng, "radius_meters": 0, "is_enabled": true,
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid radius put geofence status = %d", rec.Code)
		}
		rec = env.request(t, http.MethodPut, "/api/v1/geofence", map[string]any{
			"latitude": schoolLat, "longitude": schoolLng, "radius_meters": 300, "is_enabled": true,
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin put geofence status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var updated teacherpresencepkg.GeofenceConfig
		decode(t, rec, &updated)
		if updated.Latitude != schoolLat || updated.RadiusMeters != 300 || !updated.IsEnabled {
			t.Fatalf("updated geofence = %+v", updated)
		}
		var auditCount int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM audit_logs WHERE action = 'geofence.update' AND entity_type = 'school_geofence'",
		).Scan(&auditCount); err != nil {
			t.Fatalf("count geofence audit: %v", err)
		}
		if auditCount == 0 {
			t.Fatal("geofence change must write an audit log row")
		}
	})

	t.Run("check-in flow", func(t *testing.T) {
		rec := checkIn(t, data.teacherToken, map[string]any{
			"latitude": insideLat, "longitude": schoolLng, "accuracy": 12.5, "notes": "senin pagi",
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("check-in status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var result teacherpresencepkg.CheckInResult
		decode(t, rec, &result)
		if result.TeacherID != data.teacherID || !result.WithinGeofence {
			t.Fatalf("check-in result = %+v, teacher %s", result, data.teacherID)
		}
		if result.DistanceMeters <= 0 || result.DistanceMeters > 300 {
			t.Fatalf("check-in distance = %v, want within (0,300]", result.DistanceMeters)
		}
		if result.PresenceDate == "" || result.CheckInAt.IsZero() || result.ID == "" || result.Message == "" {
			t.Fatalf("check-in result missing fields: %+v", result)
		}

		// Same day, second check-in: conflict (unique teacher+date).
		if rec := checkIn(t, data.teacherToken, map[string]any{
			"latitude": insideLat, "longitude": schoolLng, "accuracy": 10,
		}); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate check-in status = %d, body = %s", rec.Code, rec.Body.String())
		}

		// Server-side validation of the fix.
		for name, body := range map[string]map[string]any{
			"bad latitude":   {"latitude": 100, "longitude": schoolLng, "accuracy": 10},
			"bad longitude":  {"latitude": schoolLat, "longitude": 190, "accuracy": 10},
			"negative acc":   {"latitude": schoolLat, "longitude": schoolLng, "accuracy": -1},
			"missing coords": {"accuracy": 10},
		} {
			if rec := checkIn(t, data.dutyToken, body); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s check-in status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}

		// Check-out before check-in is rejected as invalid input. When
		// "one hour ago" already falls on the previous school day the use
		// case legitimately answers conflict (no presence row for that day),
		// so the expectation follows the Jakarta-date arithmetic.
		early := time.Now().Add(-time.Hour)
		jkt, _ := time.LoadLocation("Asia/Jakarta")
		wantEarly := http.StatusBadRequest
		if early.In(jkt).Format("2006-01-02") != time.Now().In(jkt).Format("2006-01-02") {
			wantEarly = http.StatusConflict
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/teacher-presence/check-out", map[string]any{
			"latitude": insideLat, "longitude": schoolLng, "accuracy": 10, "check_time": early.Format(time.RFC3339),
		}, data.teacherToken); rec.Code != wantEarly {
			t.Fatalf("early check-out status = %d, want %d, body = %s", rec.Code, wantEarly, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/teacher-presence/check-out", map[string]any{
			"latitude": outsideLat, "longitude": schoolLng, "accuracy": 20,
		}, data.teacherToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("check-out status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var out teacherpresencepkg.CheckOutResult
		decode(t, rec, &out)
		if out.CheckOutWithinGeofence {
			t.Fatalf("check-out outside radius must report verdict false: %+v", out)
		}
		if out.CheckOutDistanceMeters < 10000 || out.DurationMinutes < 0 || out.CheckInAt.IsZero() || out.CheckOutAt.IsZero() {
			t.Fatalf("check-out result = %+v", out)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/teacher-presence/check-out", map[string]any{
			"latitude": insideLat, "longitude": schoolLng, "accuracy": 10,
		}, data.teacherToken); rec.Code != http.StatusConflict {
			t.Fatalf("double check-out status = %d, body = %s", rec.Code, rec.Body.String())
		}

		// Check-out without any check-in: conflict, not silent create.
		if rec := env.request(t, http.MethodPost, "/api/v1/teacher-presence/check-out", map[string]any{
			"latitude": insideLat, "longitude": schoolLng, "accuracy": 10,
		}, data.dutyToken); rec.Code != http.StatusConflict {
			t.Fatalf("check-out without check-in status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("geofence disabled accepts far fixes", func(t *testing.T) {
		if rec := env.request(t, http.MethodPut, "/api/v1/geofence", map[string]any{
			"latitude": schoolLat, "longitude": schoolLng, "radius_meters": 1, "is_enabled": false,
		}, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("disable geofence status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec := checkIn(t, data.dutyToken, map[string]any{
			"latitude": outsideLat, "longitude": schoolLng, "accuracy": 10,
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("far check-in status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var result teacherpresencepkg.CheckInResult
		decode(t, rec, &result)
		if !result.WithinGeofence || result.DistanceMeters < 10000 {
			t.Fatalf("disabled geofence result = %+v", result)
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/geofence", map[string]any{
			"latitude": schoolLat, "longitude": schoolLng, "radius_meters": 300, "is_enabled": true,
		}, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("re-enable geofence status = %d", rec.Code)
		}
	})

	t.Run("today and history", func(t *testing.T) {
		loc, err := time.LoadLocation("Asia/Jakarta")
		if err != nil {
			t.Fatalf("load location: %v", err)
		}
		ExpectedDate := time.Now().In(loc).Format("2006-01-02")

		rec := env.request(t, http.MethodGet, "/api/v1/teacher-presence/today", nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("today status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var today teacherpresencepkg.TodayResult
		decode(t, rec, &today)
		if today.PresenceDate != ExpectedDate || today.CheckInAt == nil || today.CheckOutAt == nil {
			t.Fatalf("today = %+v, want date %s with both stamps", today, ExpectedDate)
		}
		if today.WithinGeofence == nil || *today.WithinGeofence != true || today.DistanceMeters == nil {
			t.Fatalf("today geofence fields = %+v", today)
		}

		day := ExpectedDate
		rec = env.request(t, http.MethodGet, "/api/v1/teacher-presence/history?from="+day+"&to="+day+"&limit=10&offset=0", nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("history status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var history teacherpresencepkg.HistoryResult
		decode(t, rec, &history)
		if history.Total != 1 || len(history.Data) != 1 || history.Limit != 10 || history.Offset != 0 {
			t.Fatalf("history = %+v", history)
		}
		entry := history.Data[0]
		if entry.PresenceDate != day || !entry.WithinGeofence || entry.CheckOutAt == nil || entry.DurationMinutes < 0 {
			t.Fatalf("history entry = %+v", entry)
		}

		// Duty teacher sees only their own rows.
		rec = env.request(t, http.MethodGet, "/api/v1/teacher-presence/history", nil, data.dutyToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("duty history status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var dutyHistory teacherpresencepkg.HistoryResult
		decode(t, rec, &dutyHistory)
		if dutyHistory.Total != 1 || len(dutyHistory.Data) != 1 || dutyHistory.Data[0].TeacherID == data.teacherID {
			t.Fatalf("duty history leaked other teacher rows: %+v", dutyHistory)
		}

		if rec := env.request(t, http.MethodGet, "/api/v1/teacher-presence/history?from=bad-date", nil, data.teacherToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid date filter status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/teacher-presence/history?limit=500", nil, data.teacherToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("oversized limit status = %d", rec.Code)
		}
	})

	t.Run("presence data is teacher scoped in storage", func(t *testing.T) {
		var count int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM teacher_presences WHERE teacher_id = $1", data.teacherID,
		).Scan(&count); err != nil {
			t.Fatalf("count presences: %v", err)
		}
		if count != 1 {
			t.Fatalf("presence rows = %d, want 1", count)
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
	name := fmt.Sprintf("ifter_teacher_presence_test_%d", time.Now().UnixNano())
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
	// The pgx pool doubles as readiness; New() wires the teacher-presence
	// service from it (see router.go FeatureDeps).
	router := httptransport.New(log, pool, authService, nil, nil, nil, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	adminID := e.insertUser(t, "tp-admin", "tp-admin@example.com", "tp-admin-password", "admin")
	teacherUserID := e.insertUser(t, "tp-teacher", "tp-teacher@example.com", "tp-teacher-password", "teacher")
	dutyUserID := e.insertUser(t, "tp-duty", "tp-duty@example.com", "tp-duty-password", "duty_teacher")
	principalID := e.insertUser(t, "tp-principal", "tp-principal@example.com", "tp-principal-password", "principal")
	loneID := e.insertUser(t, "tp-lone", "tp-lone@example.com", "tp-lone-password", "teacher")
	_ = adminID
	_ = principalID
	_ = loneID
	teacherID := e.insertTeacher(t, teacherUserID, "Guru Presence")
	e.insertTeacher(t, dutyUserID, "Guru Piket Presence")
	return testData{
		adminToken:     e.login(t, "tp-admin", "tp-admin-password"),
		teacherID:      teacherID,
		teacherToken:   e.login(t, "tp-teacher", "tp-teacher-password"),
		dutyToken:      e.login(t, "tp-duty", "tp-duty-password"),
		loneToken:      e.login(t, "tp-lone", "tp-lone-password"),
		principalToken: e.login(t, "tp-principal", "tp-principal-password"),
	}
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
