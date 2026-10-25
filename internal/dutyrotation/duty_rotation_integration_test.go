package dutyrotation_test

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
	dutyrotationpkg "github.com/ghalitsar-coder/backend-ifter/internal/dutyrotation"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type testData struct {
	adminToken   string
	staffTU      string
	ownerToken   string // duty_teacher, owns the seeded assignment
	subToken     string // teacher, valid substitute target
	outsiderTok  string // duty_teacher, owns nothing, never a substitute
	principalTok string
	studentTok   string
	ownerTeach   string // teacher id of owner
	subTeach     string // teacher id of sub
	outsiderTch  string // teacher id of outsider
	inactiveTch  string // inactive teacher id
	assignment   string // duty_assignment id (ownerTeach, weekdayOfToday)
	studentID    string
	today        string // matches the assignment weekday
	nextWeek     string // today + 7 days, same weekday
}

func TestDutyRotationAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	rotationFromBody := func(t *testing.T, rec *httptest.ResponseRecorder) dutyrotationpkg.RotationItem {
		t.Helper()
		var body struct {
			DutyRotation dutyrotationpkg.RotationItem `json:"duty_rotation"`
		}
		decode(t, rec, &body)
		if body.DutyRotation.ID == "" {
			t.Fatalf("missing duty_rotation envelope: %s", rec.Body.String())
		}
		return body.DutyRotation
	}

	t.Run("authentication and rbac", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/duty-rotations", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous list status = %d", rec.Code)
		}
		// AttendanceOps = admin + teacher + duty_teacher; principal/staff_tu/student are out.
		for name, token := range map[string]string{"principal": data.principalTok, "staff_tu": data.staffTU, "student": data.studentTok} {
			if rec := env.request(t, http.MethodGet, "/api/v1/duty-rotations", nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s list status = %d", name, rec.Code)
			}
			if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", map[string]any{}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s create status = %d", name, rec.Code)
			}
		}
	})

	var rotationID string
	t.Run("admin creates a substitution", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", map[string]any{
			"duty_assignment_id":    data.assignment,
			"substitute_teacher_id": data.subTeach,
			"date":                  data.today,
			"reason":                "guru asli bertugas luar kota",
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("admin create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		item := rotationFromBody(t, rec)
		rotationID = item.ID
		if item.Status != "substituted" || item.DutyAssignmentID != data.assignment {
			t.Fatalf("created rotation = %+v", item)
		}
		if item.RotationDate != data.today || item.Weekday != isoWeekday(data.today) {
			t.Fatalf("rotation date/weekday = %s/%d, want %s/%d", item.RotationDate, item.Weekday, data.today, isoWeekday(data.today))
		}
		if item.OriginalTeacherID != data.ownerTeach || item.SubstituteTeacherID != data.subTeach {
			t.Fatalf("teacher pairing wrong: %+v", item)
		}
		if item.OriginalTeacherName == "" || item.SubstituteTeacherName == "" || item.CreatedAt.IsZero() {
			t.Fatalf("joined fields empty: %+v", item)
		}

		// Duplicate (assignment, date, substitute) triple -> 409.
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", map[string]any{
			"duty_assignment_id": data.assignment, "substitute_teacher_id": data.subTeach,
			"date": data.today, "reason": "duplikat",
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate create status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		base := map[string]any{
			"duty_assignment_id": data.assignment, "substitute_teacher_id": data.subTeach,
			"date": data.nextWeek, "reason": "alasan",
		}
		// Wrong weekday: nextWeek has the assignment weekday, so shift by one day.
		base["date"] = shiftDate(data.today, 1)
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", base, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("weekday mismatch status = %d, body = %s", rec.Code, rec.Body.String())
		}
		base["date"] = data.nextWeek
		// Outside the assignment window (start today-30 .. today+60).
		base["date"] = shiftDate(data.today, 490)
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", base, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("out of window status = %d", rec.Code)
		}
		base["date"] = data.nextWeek
		// Unknown assignment -> 404.
		base["duty_assignment_id"] = "00000000-0000-4000-8000-000000000000"
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", base, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown assignment status = %d", rec.Code)
		}
		base["duty_assignment_id"] = data.assignment
		// Unknown substitute -> 404; inactive substitute -> 400; self -> 400.
		base["substitute_teacher_id"] = "00000000-0000-4000-8000-000000000000"
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", base, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown substitute status = %d", rec.Code)
		}
		base["substitute_teacher_id"] = data.inactiveTch
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", base, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("inactive substitute status = %d", rec.Code)
		}
		base["substitute_teacher_id"] = data.ownerTeach
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", base, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("self substitution status = %d", rec.Code)
		}
		base["substitute_teacher_id"] = data.subTeach
		// Blank reason and malformed ids/dates -> 400.
		base["reason"] = "   "
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", base, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("blank reason status = %d", rec.Code)
		}
		base["reason"] = "alasan"
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", map[string]any{
			"duty_assignment_id": "not-a-uuid", "substitute_teacher_id": data.subTeach,
			"date": data.nextWeek, "reason": "alasan",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad assignment id status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", map[string]any{
			"duty_assignment_id": data.assignment, "substitute_teacher_id": data.subTeach,
			"date": "2026/13/01", "reason": "alasan",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad date status = %d", rec.Code)
		}
	})

	t.Run("duty_teacher ownership", func(t *testing.T) {
		// The assignment owner may substitute their own duty.
		rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", map[string]any{
			"duty_assignment_id": data.assignment, "substitute_teacher_id": data.subTeach,
			"date": data.nextWeek, "reason": "kendala keluarga",
		}, data.ownerToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("owner create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// An unaffiliated duty_teacher may not touch someone else's assignment.
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-rotations", map[string]any{
			"duty_assignment_id": data.assignment, "substitute_teacher_id": data.ownerTeach,
			"date": shiftDate(data.nextWeek, 7), "reason": "bukan piket saya",
		}, data.outsiderTok); rec.Code != http.StatusForbidden {
			t.Fatalf("outsider create status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("list scope and filters", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/duty-rotations", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var all dutyrotationpkg.RotationList
		decode(t, rec, &all)
		if len(all.Data) != 2 {
			t.Fatalf("admin rotations = %d, want 2", len(all.Data))
		}
		// The named substitute (teacher role) sees substitutions involving them.
		rec = env.request(t, http.MethodGet, "/api/v1/duty-rotations", nil, data.subToken)
		var mine dutyrotationpkg.RotationList
		decode(t, rec, &mine)
		if len(mine.Data) != 2 {
			t.Fatalf("substitute sees %d rotations, want 2", len(mine.Data))
		}
		for _, item := range mine.Data {
			if item.SubstituteTeacherID != data.subTeach {
				t.Fatalf("scope leaked to non-relevant rotation: %+v", item)
			}
		}
		// The outsider sees nothing.
		rec = env.request(t, http.MethodGet, "/api/v1/duty-rotations", nil, data.outsiderTok)
		var none dutyrotationpkg.RotationList
		decode(t, rec, &none)
		if len(none.Data) != 0 {
			t.Fatalf("outsider sees %d rotations, want 0", len(none.Data))
		}
		// date filter.
		rec = env.request(t, http.MethodGet, "/api/v1/duty-rotations?date="+data.today, nil, data.adminToken)
		var dated dutyrotationpkg.RotationList
		decode(t, rec, &dated)
		if len(dated.Data) != 1 || dated.Data[0].RotationDate != data.today {
			t.Fatalf("date filter = %+v", dated.Data)
		}
		// weekday filter (assignment weekday).
		rec = env.request(t, http.MethodGet, "/api/v1/duty-rotations?weekday="+fmt.Sprint(isoWeekday(data.today)), nil, data.adminToken)
		var wd dutyrotationpkg.RotationList
		decode(t, rec, &wd)
		if len(wd.Data) != 2 {
			t.Fatalf("weekday filter = %+v", wd.Data)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/duty-rotations?weekday=0", nil, data.adminToken)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("bad weekday status = %d", rec.Code)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/duty-rotations?date=nope", nil, data.adminToken)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("bad date status = %d", rec.Code)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		// Outsider duty_teacher may not cancel someone else's substitution.
		if rec := env.request(t, http.MethodPatch, "/api/v1/duty-rotations/"+rotationID+"/cancel",
			map[string]any{"reason": "bukan ranah saya"}, data.outsiderTok); rec.Code != http.StatusForbidden {
			t.Fatalf("outsider cancel status = %d", rec.Code)
		}
		// The substitute (teacher) cancels their own — audited, 200.
		rec := env.request(t, http.MethodPatch, "/api/v1/duty-rotations/"+rotationID+"/cancel",
			map[string]any{"reason": "pembatalan mendadak"}, data.subToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("substitute cancel status = %d, body = %s", rec.Code, rec.Body.String())
		}
		item := rotationFromBody(t, rec)
		if item.Status != "cancelled" || item.Reason != "pembatalan mendadak" {
			t.Fatalf("cancelled item = %+v", item)
		}
		// Double cancel -> 409.
		if rec := env.request(t, http.MethodPatch, "/api/v1/duty-rotations/"+rotationID+"/cancel",
			map[string]any{}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("second cancel status = %d", rec.Code)
		}
		// Unknown rotation -> 404; malformed id -> 400.
		if rec := env.request(t, http.MethodPatch, "/api/v1/duty-rotations/00000000-0000-4000-8000-000000000000/cancel",
			map[string]any{}, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown cancel status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPatch, "/api/v1/duty-rotations/not-a-uuid/cancel",
			map[string]any{}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad id cancel status = %d", rec.Code)
		}
	})

	t.Run("substitutions are audited", func(t *testing.T) {
		var createCount, cancelCount int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM audit_logs WHERE action = 'duty.substitution' AND entity_type = 'duty_rotation'",
		).Scan(&createCount); err != nil {
			t.Fatalf("count substitution audit: %v", err)
		}
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM audit_logs WHERE action = 'duty.substitution.cancel' AND entity_type = 'duty_rotation'",
		).Scan(&cancelCount); err != nil {
			t.Fatalf("count cancel audit: %v", err)
		}
		if createCount < 2 {
			t.Fatalf("duty.substitution audit rows = %d, want >= 2", createCount)
		}
		if cancelCount < 1 {
			t.Fatalf("duty.substitution.cancel audit rows = %d, want >= 1", cancelCount)
		}
	})

	t.Run("uniform reports rbac and creation", func(t *testing.T) {
		// uniform = admin + staff_tu + duty_teacher (package spec RBAC).
		if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
			"for_date": data.today, "teacher_id": data.ownerTeach, "scope": "teacher",
		}, data.adminToken); rec.Code != http.StatusCreated {
			t.Fatalf("admin uniform create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
			"for_date": data.today, "teacher_id": data.ownerTeach, "scope": "student",
			"target_student_id": data.studentID, "note": "dasi tidak terpakai",
		}, data.staffTU); rec.Code != http.StatusCreated {
			t.Fatalf("staff_tu uniform create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
			"for_date": data.nextWeek, "teacher_id": data.ownerTeach, "scope": "student",
		}, data.ownerToken); rec.Code != http.StatusCreated {
			t.Fatalf("duty_teacher uniform create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		for name, token := range map[string]string{"teacher": data.subToken, "principal": data.principalTok, "student": data.studentTok} {
			if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
				"for_date": data.today, "teacher_id": data.ownerTeach, "scope": "teacher",
			}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s uniform create status = %d", name, rec.Code)
			}
		}

		// Validation.
		if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
			"for_date": data.today, "teacher_id": data.ownerTeach, "scope": "teacher",
			"target_student_id": data.studentID,
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("teacher scope with student target status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
			"for_date": data.today, "teacher_id": data.ownerTeach, "scope": "seragam",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid scope status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
			"for_date": data.today, "teacher_id": "00000000-0000-4000-8000-000000000000", "scope": "teacher",
		}, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown teacher status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
			"for_date": data.today, "teacher_id": data.ownerTeach, "scope": "student",
			"target_student_id": "00000000-0000-4000-8000-000000000000",
		}, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown student status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/uniform-reports", map[string]any{
			"for_date": "01-13-2026", "teacher_id": data.ownerTeach, "scope": "teacher",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad for_date status = %d", rec.Code)
		}
	})

	t.Run("uniform reports list filters", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/uniform-reports", nil, data.staffTU)
		if rec.Code != http.StatusOK {
			t.Fatalf("uniform list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var all dutyrotationpkg.UniformList
		decode(t, rec, &all)
		if len(all.Data) != 3 {
			t.Fatalf("uniform rows = %d, want 3", len(all.Data))
		}
		rec = env.request(t, http.MethodGet, "/api/v1/uniform-reports?date="+data.today+"&teacher_id="+data.ownerTeach, nil, data.adminToken)
		var dated dutyrotationpkg.UniformList
		decode(t, rec, &dated)
		if len(dated.Data) != 2 {
			t.Fatalf("date+teacher filter = %+v", dated.Data)
		}
		for _, item := range dated.Data {
			if item.ForDate != data.today || item.TeacherID != data.ownerTeach {
				t.Fatalf("filter leaked: %+v", item)
			}
		}
		// Nullable fields round-trip as null when absent.
		if raw := rec.Body.String(); !strings.Contains(raw, `"target_student_id":null`) {
			t.Fatalf("response missing null target_student_id: %s", raw)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/uniform-reports?date=nope", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad date status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/uniform-reports?teacher_id=nope", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad teacher_id status = %d", rec.Code)
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
	name := fmt.Sprintf("ifter_dutyrotation_test_%d", time.Now().UnixNano())
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
	// The pgx pool doubles as readiness; New() wires the duty-rotation service
	// from it (see router.go FeatureDeps).
	router := httptransport.New(log, pool, authService, nil, nil, nil, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	now := time.Now().UTC().Truncate(24 * time.Hour)
	today := now.Format("2006-01-02")

	ownerUserID := e.insertUser(t, "dutyrot-owner", "dutyrot-owner@example.com", "dutyrot-owner-pw", "duty_teacher")
	subUserID := e.insertUser(t, "dutyrot-sub", "dutyrot-sub@example.com", "dutyrot-sub-pw", "teacher")
	outsiderUserID := e.insertUser(t, "dutyrot-out", "dutyrot-out@example.com", "dutyrot-out-pw", "duty_teacher")
	inactiveUserID := e.insertUser(t, "dutyrot-inact", "dutyrot-inact@example.com", "dutyrot-inact-pw", "teacher")
	staffID := e.insertUser(t, "dutyrot-staff", "dutyrot-staff@example.com", "dutyrot-staff-pw", "staff_tu")
	principalID := e.insertUser(t, "dutyrot-principal", "dutyrot-principal@example.com", "dutyrot-principal-pw", "principal")
	studentUserID := e.insertUser(t, "dutyrot-siswa", "dutyrot-siswa@example.com", "dutyrot-siswa-pw", "student")
	_, _, _ = staffID, principalID, studentUserID

	ownerTeach := e.insertTeacher(t, ownerUserID, "Guru Piket Owner", "active")
	subTeach := e.insertTeacher(t, subUserID, "Guru Pengganti", "active")
	outsiderTch := e.insertTeacher(t, outsiderUserID, "Guru Piket Lain", "active")
	inactiveTch := e.insertTeacher(t, inactiveUserID, "Guru Nonaktif", "inactive")

	start := now.AddDate(0, 0, -30).Format("2006-01-02")
	end := now.AddDate(0, 0, 60).Format("2006-01-02")
	weekday := isoWeekday(today)
	adminID := e.insertUser(t, "dutyrot-admin", "dutyrot-admin@example.com", "dutyrot-admin-pw", "admin")
	assignment := e.insert(t,
		`INSERT INTO duty_assignments (teacher_id, weekday, start_date, end_date, created_by)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
		ownerTeach, weekday, start, end, adminID)

	studentID := e.insert(t,
		`INSERT INTO students (student_number, full_name, admission_year)
		 VALUES ('DUTYROT-NIS', 'Siswa Seragam', 2026) RETURNING id::text`,
	)

	return testData{
		adminToken:   e.login(t, "dutyrot-admin", "dutyrot-admin-pw"),
		staffTU:      e.login(t, "dutyrot-staff", "dutyrot-staff-pw"),
		ownerToken:   e.login(t, "dutyrot-owner", "dutyrot-owner-pw"),
		subToken:     e.login(t, "dutyrot-sub", "dutyrot-sub-pw"),
		outsiderTok:  e.login(t, "dutyrot-out", "dutyrot-out-pw"),
		principalTok: e.login(t, "dutyrot-principal", "dutyrot-principal-pw"),
		studentTok:   e.login(t, "dutyrot-siswa", "dutyrot-siswa-pw"),
		ownerTeach:   ownerTeach,
		subTeach:     subTeach,
		outsiderTch:  outsiderTch,
		inactiveTch:  inactiveTch,
		assignment:   assignment,
		studentID:    studentID,
		today:        today,
		nextWeek:     shiftDate(today, 7),
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

func (e apiEnv) insertTeacher(t *testing.T, userID, name, status string) string {
	t.Helper()
	return e.insert(t,
		`INSERT INTO teachers (user_id, full_name, status) VALUES ($1,$2,$3) RETURNING id::text`,
		userID, name, status)
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

func shiftDate(day string, delta int) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}
	return t.AddDate(0, 0, delta).Format("2006-01-02")
}

// isoWeekday maps YYYY-MM-DD to 1..7 (Mon..Sun), the duty_assignments.weekday
// domain used across the attendance package.
func isoWeekday(day string) int {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}
	return (int(t.Weekday())+6)%7 + 1
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
