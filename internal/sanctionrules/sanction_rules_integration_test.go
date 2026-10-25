package sanctionrules_test

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
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	disciplinepkg "github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	sanctionrulespkg "github.com/ghalitsar-coder/backend-ifter/internal/sanctionrules"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type testData struct {
	adminToken     string
	teacherToken   string
	principalToken string
	staffTUToken   string
	studentWithPts string // gets two 6-point records -> total 12
	studentClean   string // no records -> total 0
}

func TestSanctionRulesAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	ruleFromEnvelope := func(t *testing.T, rec *httptest.ResponseRecorder) sanctionrulespkg.Rule {
		t.Helper()
		var body struct {
			SanctionRule sanctionrulespkg.Rule `json:"sanction_rule"`
		}
		decode(t, rec, &body)
		if body.SanctionRule.ID == "" {
			t.Fatalf("missing sanction_rule envelope: %s", rec.Body.String())
		}
		return body.SanctionRule
	}

	t.Run("authentication and rbac", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/sanction-rules", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous list status = %d", rec.Code)
		}
		// DisciplineRead = admin+teacher+duty_teacher+principal; staff_tu and
		// student are out of every sanction-rule group.
		for name, token := range map[string]string{"staff_tu": data.staffTUToken} {
			if rec := env.request(t, http.MethodGet, "/api/v1/sanction-rules", nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s list status = %d", name, rec.Code)
			}
		}
		for name, token := range map[string]string{"teacher": data.teacherToken, "principal": data.principalToken} {
			if rec := env.request(t, http.MethodGet, "/api/v1/sanction-rules", nil, token); rec.Code != http.StatusOK {
				t.Fatalf("%s list status = %d", name, rec.Code)
			}
			// Writes are admin-only.
			if rec := env.request(t, http.MethodPost, "/api/v1/sanction-rules", map[string]any{
				"name": "x", "min_points": 1, "action": "y",
			}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s create status = %d", name, rec.Code)
			}
			if rec := env.request(t, http.MethodPut, "/api/v1/sanction-rules", map[string]any{"rules": []any{}}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s replace-all status = %d", name, rec.Code)
			}
		}
	})

	var ruleA string
	t.Run("single-rule lifecycle", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/sanction-rules", map[string]any{
			"name": "Teguran lisan", "min_points": 0, "max_points": 9,
			"action": "Teguran lisan oleh wali kelas", "level": "ringan", "description": "Pelanggaran ringan",
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create A status = %d, body = %s", rec.Code, rec.Body.String())
		}
		created := ruleFromEnvelope(t, rec)
		ruleA = created.ID
		if created.MinPoints != 0 || created.MaxPoints == nil || *created.MaxPoints != 9 ||
			created.Level == nil || *created.Level != "ringan" || !created.IsActive ||
			created.Description == nil || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
			t.Fatalf("created rule = %+v", created)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/sanction-rules", map[string]any{
			"name": "Tumpang tindih", "min_points": 5, "max_points": 12, "action": "A",
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("overlapping create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// Open-ended second rule, no overlap with [0,9].
		rec = env.request(t, http.MethodPost, "/api/v1/sanction-rules", map[string]any{
			"name": "Pemanggilan orang tua", "min_points": 10,
			"action": "Pemanggilan orang tua", "level": "sedang",
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create B status = %d, body = %s", rec.Code, rec.Body.String())
		}
		ruleB := ruleFromEnvelope(t, rec).ID

		rec = env.request(t, http.MethodGet, "/api/v1/sanction-rules", nil, data.principalToken)
		var list sanctionrulespkg.RuleList
		decode(t, rec, &list)
		if len(list.Data) != 2 || list.Data[0].MinPoints != 0 || list.Data[1].MinPoints != 10 {
			t.Fatalf("list = %+v", list.Data)
		}
		if list.Data[1].MaxPoints != nil || list.Data[1].Level == nil || *list.Data[1].Level != "sedang" {
			t.Fatalf("open-ended rule = %+v", list.Data[1])
		}
		if list.Data[0].Description == nil {
			t.Fatalf("description null for rule carrying one: %+v", list.Data[0])
		}

		// PUT /:id updates the payload, keeps is_active, and rejects an
		// overlapping range move (10 -> 4 collides with [0,9]).
		if rec := env.request(t, http.MethodPut, "/api/v1/sanction-rules/"+ruleB, map[string]any{
			"name": "Geser bertabrakan", "min_points": 4, "action": "B", "level": "sedang",
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("overlapping update status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPut, "/api/v1/sanction-rules/"+ruleA, map[string]any{
			"name": "Teguran tertulis", "min_points": 0, "max_points": 9,
			"action": "Surat peringatan", "level": "ringan",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("update A status = %d, body = %s", rec.Code, rec.Body.String())
		}
		updated := ruleFromEnvelope(t, rec)
		if updated.Action != "Surat peringatan" || updated.Name != "Teguran tertulis" || !updated.IsActive {
			t.Fatalf("updated A = %+v", updated)
		}

		// PATCH status: deactivate, reactivate (no overlap appears meanwhile).
		rec = env.request(t, http.MethodPatch, "/api/v1/sanction-rules/"+ruleA+"/status",
			map[string]bool{"is_active": false}, data.adminToken)
		if rec.Code != http.StatusOK || ruleFromEnvelope(t, rec).IsActive {
			t.Fatalf("deactivate A status/body = %+v", rec)
		}
		// With A inactive, B may move down to [4,9] without colliding.
		if rec := env.request(t, http.MethodPut, "/api/v1/sanction-rules/"+ruleB, map[string]any{
			"name": "Pemanggilan orang tua", "min_points": 4, "max_points": 9,
			"action": "Pemanggilan orang tua", "level": "sedang",
		}, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("move B under inactive A status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// Reactivating A [0,9] now collides with active B [4,9] -> 409.
		if rec := env.request(t, http.MethodPatch, "/api/v1/sanction-rules/"+ruleA+"/status",
			map[string]bool{"is_active": true}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("reactivate overlapping A status = %d, body = %s", rec.Code, rec.Body.String())
		}

		// DELETE + 404 afterwards.
		if rec := env.request(t, http.MethodDelete, "/api/v1/sanction-rules/"+ruleA, nil, data.adminToken); rec.Code != http.StatusNoContent {
			t.Fatalf("delete A status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/sanction-rules/"+ruleA, nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("get deleted A status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodDelete, "/api/v1/sanction-rules/"+ruleA, nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("re-delete A status = %d", rec.Code)
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		cases := []struct {
			name string
			body map[string]any
		}{
			{"missing min_points", map[string]any{"name": "x", "action": "y"}},
			{"negative min", map[string]any{"name": "x", "min_points": -1, "action": "y"}},
			{"max equals min", map[string]any{"name": "x", "min_points": 5, "max_points": 5, "action": "y"}},
			{"max below min", map[string]any{"name": "x", "min_points": 20, "max_points": 10, "action": "y"}},
			{"missing action", map[string]any{"name": "x", "min_points": 0}},
			{"blank name is derived but action still required", map[string]any{"name": "  ", "min_points": 0, "action": "  "}},
		}
		for _, tt := range cases {
			t.Run(tt.name, func(t *testing.T) {
				if rec := env.request(t, http.MethodPost, "/api/v1/sanction-rules", tt.body, data.adminToken); rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
				}
			})
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/sanction-rules/not-a-uuid", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad id status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/sanction-rules", []byte(`{"name":`), data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed json status = %d", rec.Code)
		}
	})

	t.Run("replace-all swaps the whole set", func(t *testing.T) {
		rules := []map[string]any{
			{"min_points": 10, "max_points": 19, "action": "Pemanggilan orang tua", "level": "sedang", "is_active": true},
			{"min_points": 20, "action": "Pembinaan kesiswaan", "level": "berat", "is_active": true},
			{"min_points": 5, "max_points": 9, "action": "Catatan wali kelas", "level": "ringan", "is_active": false},
		}
		rec := env.request(t, http.MethodPut, "/api/v1/sanction-rules", map[string]any{"rules": rules}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("replace-all status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list sanctionrulespkg.RuleList
		decode(t, rec, &list)
		if len(list.Data) != 3 {
			t.Fatalf("replaced set = %+v", list.Data)
		}
		if list.Data[0].MinPoints != 5 || list.Data[0].Name != "Catatan wali kelas" {
			t.Fatalf("replace-all not ordered by ladder or name not derived: %+v", list.Data[0])
		}
		if list.Data[2].MaxPoints != nil {
			t.Fatalf("top tier should be open-ended: %+v", list.Data[2])
		}
		// The previous set (rule B) is gone.
		rec = env.request(t, http.MethodGet, "/api/v1/sanction-rules", nil, data.adminToken)
		decode(t, rec, &list)
		if len(list.Data) != 3 {
			t.Fatalf("set after replace = %+v", list.Data)
		}
		// Overlapping submitted set -> 409 and the table stays untouched.
		if rec := env.request(t, http.MethodPut, "/api/v1/sanction-rules", map[string]any{
			"rules": []map[string]any{
				{"min_points": 10, "max_points": 20, "action": "A", "level": "x", "is_active": true},
				{"min_points": 15, "action": "B", "level": "y", "is_active": true},
			},
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("overlapping replace-all status = %d", rec.Code)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/sanction-rules", nil, data.adminToken)
		decode(t, rec, &list)
		if len(list.Data) != 3 {
			t.Fatalf("failed replace mutated the set: %+v", list.Data)
		}
		// Missing rules key -> 400.
		if rec := env.request(t, http.MethodPut, "/api/v1/sanction-rules", map[string]any{}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("missing rules status = %d", rec.Code)
		}
	})

	t.Run("student summary carries current_level", func(t *testing.T) {
		// Category worth 6 points; two records put the student at total 12,
		// inside the active [10,19] tier. The clean student (0 points) matches
		// nothing: [5,9] is inactive and no rule covers 0.
		rec := env.request(t, http.MethodPost, "/api/v1/discipline/categories", map[string]any{
			"name": "Sanction test telat", "default_points": 6,
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create category status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var category disciplinepkg.Category
		decode(t, rec, &category)
		for i := 0; i < 2; i++ {
			rec = env.request(t, http.MethodPost, "/api/v1/discipline/records", map[string]any{
				"student_id": data.studentWithPts, "category_id": category.ID,
				"occurred_at": "2026-09-02T09:00:00+07:00",
				"description": fmt.Sprintf("Pelanggaran %d", i+1),
			}, data.adminToken)
			if rec.Code != http.StatusCreated {
				t.Fatalf("create record status = %d, body = %s", rec.Code, rec.Body.String())
			}
		}
		rec = env.request(t, http.MethodGet, "/api/v1/discipline/students/"+data.studentWithPts, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var summary disciplinepkg.StudentSummary
		decode(t, rec, &summary)
		if summary.TotalPoints != 12 {
			t.Fatalf("total points = %d, want 12", summary.TotalPoints)
		}
		if summary.CurrentLevel == nil || summary.CurrentLevel.Level != "sedang" ||
			summary.CurrentLevel.Action != "Pemanggilan orang tua" {
			t.Fatalf("current_level = %+v, want sedang tier", summary.CurrentLevel)
		}
		if raw := rec.Body.String(); !strings.Contains(raw, `"current_level"`) {
			t.Fatalf("current_level key missing: %s", raw)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/discipline/students/"+data.studentClean, nil, data.adminToken)
		decode(t, rec, &summary)
		if summary.CurrentLevel != nil {
			t.Fatalf("clean student current_level = %+v, want null", summary.CurrentLevel)
		}
		if raw := rec.Body.String(); !strings.Contains(raw, `"current_level":null`) {
			t.Fatalf("response missing null current_level: %s", raw)
		}
	})

	t.Run("writes are audited", func(t *testing.T) {
		counts := func(action string) int {
			var n int
			if err := env.pool.QueryRow(context.Background(),
				"SELECT count(*) FROM audit_logs WHERE action = $1 AND entity_type = 'sanction_rule'", action,
			).Scan(&n); err != nil {
				t.Fatalf("count %s audit: %v", action, err)
			}
			return n
		}
		if n := counts("sanction_rule.create"); n < 2 {
			t.Fatalf("create audit rows = %d, want >= 2", n)
		}
		if n := counts("sanction_rule.update"); n < 1 {
			t.Fatalf("update audit rows = %d, want >= 1", n)
		}
		if n := counts("sanction_rule.status"); n < 1 {
			t.Fatalf("status audit rows = %d, want >= 1", n)
		}
		if n := counts("sanction_rule.delete"); n != 1 {
			t.Fatalf("delete audit rows = %d, want 1", n)
		}
		if n := counts("sanction_rule.replace"); n != 1 {
			t.Fatalf("replace audit rows = %d, want 1 (the 409 replace must not audit)", n)
		}
		// The replace audit carries before/after rule counts (package spec).
		var before, after string
		if err := env.pool.QueryRow(context.Background(),
			`SELECT before_data->>'rule_count', after_data->>'rule_count'
			 FROM audit_logs WHERE action = 'sanction_rule.replace' ORDER BY created_at DESC LIMIT 1`,
		).Scan(&before, &after); err != nil {
			t.Fatalf("read replace audit: %v", err)
		}
		if before != "1" || after != "3" {
			t.Fatalf("replace before/after counts = %s/%s, want 1/3", before, after)
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
	name := fmt.Sprintf("ifter_sanctionrules_test_%d", time.Now().UnixNano())
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
	// The pgx pool doubles as readiness; New() wires the sanction-rules
	// service from it (see router.go FeatureDeps). The discipline service is
	// needed for the current_level surface on GET /discipline/students/:id.
	router := httptransport.New(log, pool, authService, nil, nil, nil, disciplinepkg.NewService(pool, log), nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	e.insertUser(t, "sanct-admin", "sanct-admin@example.com", "sanct-admin-pw", "admin")
	e.insertUser(t, "sanct-teacher", "sanct-teacher@example.com", "sanct-teacher-pw", "teacher")
	e.insertUser(t, "sanct-principal", "sanct-principal@example.com", "sanct-principal-pw", "principal")
	e.insertUser(t, "sanct-staff", "sanct-staff@example.com", "sanct-staff-pw", "staff_tu")
	e.insertUser(t, "sanct-siswa", "sanct-siswa@example.com", "sanct-siswa-pw", "student")

	yearID := e.insert(t,
		`INSERT INTO academic_years (name, start_date, end_date, is_active)
		 VALUES ('2026/2027', '2026-07-01', '2027-06-30', true) RETURNING id::text`)
	classID := e.insert(t,
		`INSERT INTO classes (academic_year_id, name, grade_level, major)
		 VALUES ($1, '10A', 10, 'IPA') RETURNING id::text`, yearID)
	_ = classID
	withPts := e.insert(t,
		`INSERT INTO students (student_number, full_name, admission_year, status)
		 VALUES ('SCT-001', 'Ani Berpoin', 2026, 'active') RETURNING id::text`)
	clean := e.insert(t,
		`INSERT INTO students (student_number, full_name, admission_year, status)
		 VALUES ('SCT-002', 'Budi Bersih', 2026, 'active') RETURNING id::text`)
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO class_enrollments (student_id, class_id, start_date) VALUES ($1,$2,'2026-07-01')`,
		withPts, classID,
	); err != nil {
		t.Fatalf("insert enrollment: %v", err)
	}

	return testData{
		adminToken:     e.login(t, "sanct-admin", "sanct-admin-pw"),
		teacherToken:   e.login(t, "sanct-teacher", "sanct-teacher-pw"),
		principalToken: e.login(t, "sanct-principal", "sanct-principal-pw"),
		staffTUToken:   e.login(t, "sanct-staff", "sanct-staff-pw"),
		studentWithPts: withPts,
		studentClean:   clean,
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
