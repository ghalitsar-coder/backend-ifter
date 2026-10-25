package devices_test

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
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type testData struct {
	adminToken     string
	staffTUToken   string
	teacherToken   string
	principalToken string
	studentToken   string
}

func TestDevicesAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	deviceFromEnvelope := func(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var body struct {
			Device map[string]any `json:"device"`
		}
		decode(t, rec, &body)
		if body.Device == nil || body.Device["id"] == "" {
			t.Fatalf("missing device envelope: %s", rec.Body.String())
		}
		return body.Device
	}

	t.Run("authentication and rbac", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/devices", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous list status = %d", rec.Code)
		}
		for name, token := range map[string]string{
			"staff_tu":  data.staffTUToken,
			"teacher":   data.teacherToken,
			"principal": data.principalToken,
			"student":   data.studentToken,
		} {
			if rec := env.request(t, http.MethodGet, "/api/v1/devices", nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s list status = %d", name, rec.Code)
			}
			if rec := env.request(t, http.MethodPost, "/api/v1/devices", map[string]any{
				"name": "x", "identifier": "x", "device_type": "gate",
			}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s create status = %d", name, rec.Code)
			}
		}
	})

	var readerID, tabletID string
	t.Run("create and read", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/devices", map[string]any{
			"name": "Reader Gerbang Utama", "identifier": "RFID-GATE-01",
			"device_type": "rfid_reader", "location": "Gerbang depan",
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		created := deviceFromEnvelope(t, rec)
		readerID, _ = created["id"].(string)
		if created["name"] != "Reader Gerbang Utama" ||
			created["identifier"] != "RFID-GATE-01" ||
			created["device_type"] != "rfid_reader" ||
			created["location"] != "Gerbang depan" ||
			created["notes"] != nil ||
			created["status"] != "active" ||
			created["last_seen_at"] != nil {
			t.Fatalf("created device = %+v", created)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/devices/"+readerID, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("get status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := deviceFromEnvelope(t, rec); got["id"] != readerID {
			t.Fatalf("get device = %+v", got)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/devices", map[string]any{
			"name": "Tablet Piket", "identifier": "TAB-01", "device_type": "tablet",
			"notes": "Dipakai piket harian",
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create tablet status = %d, body = %s", rec.Code, rec.Body.String())
		}
		tabletID, _ = deviceFromEnvelope(t, rec)["id"].(string)

		if rec := env.request(t, http.MethodGet, "/api/v1/devices/not-a-uuid", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed id status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/devices/00000000-0000-0000-0000-000000000000", nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown id status = %d", rec.Code)
		}
	})

	t.Run("validation and uniqueness", func(t *testing.T) {
		// Case-insensitive identifier duplicate.
		rec := env.request(t, http.MethodPost, "/api/v1/devices", map[string]any{
			"name": "Reader Duplikat", "identifier": "rfid-gate-01", "device_type": "rfid_reader",
		}, data.adminToken)
		if rec.Code != http.StatusConflict {
			t.Fatalf("duplicate identifier status = %d, body = %s", rec.Code, rec.Body.String())
		}
		for name, body := range map[string]map[string]any{
			"blank name":     {"name": "  ", "identifier": "X-1", "device_type": "gate"},
			"blank id":       {"name": "Gate", "identifier": " ", "device_type": "gate"},
			"bad type":       {"name": "Gate", "identifier": "X-2", "device_type": "printer"},
			"oversize name":  {"name": string(bytes.Repeat([]byte("a"), 101)), "identifier": "X-3", "device_type": "gate"},
			"oversize notes": {"name": "Gate", "identifier": "X-4", "device_type": "gate", "notes": string(bytes.Repeat([]byte("n"), 2001))},
		} {
			if rec := env.request(t, http.MethodPost, "/api/v1/devices", body, data.adminToken); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("update and filters", func(t *testing.T) {
		rec := env.request(t, http.MethodPut, "/api/v1/devices/"+tabletID, map[string]any{
			"name": "Tablet Piket Depan", "identifier": "TAB-01", "device_type": "tablet",
			"location": "Ruang Piket", "notes": "",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
		}
		updated := deviceFromEnvelope(t, rec)
		if updated["name"] != "Tablet Piket Depan" || updated["location"] != "Ruang Piket" || updated["notes"] != nil {
			t.Fatalf("updated device = %+v", updated)
		}

		// Moving onto another device's identifier conflicts.
		if rec := env.request(t, http.MethodPut, "/api/v1/devices/"+tabletID, map[string]any{
			"name": "Tablet", "identifier": "RFID-GATE-01", "device_type": "tablet",
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("update identifier conflict status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/devices/00000000-0000-0000-0000-000000000000", map[string]any{
			"name": "Ghost", "identifier": "GHOST", "device_type": "other",
		}, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("update unknown status = %d", rec.Code)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/devices?type=tablet", nil, data.adminToken)
		var list struct {
			Data []map[string]any `json:"data"`
		}
		decode(t, rec, &list)
		if len(list.Data) != 1 || list.Data[0]["id"] != tabletID {
			t.Fatalf("type filter = %+v", list.Data)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/devices?type=printer", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid type filter status = %d", rec.Code)
		}
	})

	t.Run("status is a soft retirement", func(t *testing.T) {
		rec := env.request(t, http.MethodPatch, "/api/v1/devices/"+readerID+"/status", map[string]any{
			"status": "inactive",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("status patch = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := deviceFromEnvelope(t, rec); got["status"] != "inactive" {
			t.Fatalf("patched device = %+v", got)
		}
		if rec := env.request(t, http.MethodPatch, "/api/v1/devices/"+readerID+"/status", map[string]any{
			"status": "retired",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid status patch = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPatch, "/api/v1/devices/00000000-0000-0000-0000-000000000000/status", map[string]any{
			"status": "active",
		}, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown status patch = %d", rec.Code)
		}

		var list struct {
			Data []map[string]any `json:"data"`
		}
		rec = env.request(t, http.MethodGet, "/api/v1/devices?status=inactive", nil, data.adminToken)
		decode(t, rec, &list)
		if len(list.Data) != 1 || list.Data[0]["id"] != readerID {
			t.Fatalf("inactive filter = %+v", list.Data)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/devices?status=active", nil, data.adminToken)
		decode(t, rec, &list)
		if len(list.Data) != 1 || list.Data[0]["id"] != tabletID {
			t.Fatalf("active filter = %+v", list.Data)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/devices?status=broken", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid status filter = %d", rec.Code)
		}

		// No delete surface: retirement is the only removal path.
		if rec := env.request(t, http.MethodDelete, "/api/v1/devices/"+readerID, nil, data.adminToken); rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("delete status = %d", rec.Code)
		}
	})

	t.Run("writes are audited", func(t *testing.T) {
		rows, err := env.pool.Query(context.Background(),
			`SELECT action, entity_type, entity_id::text
			 FROM audit_logs WHERE entity_type = 'device' ORDER BY created_at, id`)
		if err != nil {
			t.Fatalf("query audit logs: %v", err)
		}
		defer rows.Close()
		var actions []string
		for rows.Next() {
			var action, entityType, entityID string
			if err := rows.Scan(&action, &entityType, &entityID); err != nil {
				t.Fatalf("scan audit row: %v", err)
			}
			actions = append(actions, action)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate audit rows: %v", err)
		}
		want := []string{"device.create", "device.create", "device.update", "device.status"}
		if len(actions) != len(want) {
			t.Fatalf("audit actions = %v, want %v", actions, want)
		}
		for i := range want {
			if actions[i] != want[i] {
				t.Fatalf("audit actions = %v, want %v", actions, want)
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
	name := fmt.Sprintf("ifter_devices_test_%d", time.Now().UnixNano())
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
	e.insertUser(t, "dev-admin", "dev-admin@example.com", "dev-admin-pw", "admin")
	e.insertUser(t, "dev-staff", "dev-staff@example.com", "dev-staff-pw", "staff_tu")
	e.insertUser(t, "dev-teacher", "dev-teacher@example.com", "dev-teacher-pw", "teacher")
	e.insertUser(t, "dev-principal", "dev-principal@example.com", "dev-principal-pw", "principal")
	e.insertUser(t, "dev-student", "dev-student@example.com", "dev-student-pw", "student")

	return testData{
		adminToken:     e.login(t, "dev-admin", "dev-admin-pw"),
		staffTUToken:   e.login(t, "dev-staff", "dev-staff-pw"),
		teacherToken:   e.login(t, "dev-teacher", "dev-teacher-pw"),
		principalToken: e.login(t, "dev-principal", "dev-principal-pw"),
		studentToken:   e.login(t, "dev-student", "dev-student-pw"),
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

func decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
