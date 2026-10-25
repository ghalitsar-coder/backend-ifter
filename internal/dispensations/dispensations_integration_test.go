package dispensations_test

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
	"golang.org/x/crypto/bcrypt"

	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	dispensationpkg "github.com/ghalitsar-coder/backend-ifter/internal/dispensations"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
	dir    string
}

type testData struct {
	adminToken     string
	staffTU        string
	homeroomToken  string
	otherTeach     string
	dutyToken      string
	principalToken string
	studentToken   string
	classA         string
	classB         string
	studentA       string // enrolled in class A
	studentB       string // enrolled in class B
	today          string
}

func TestDispensationsAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	itemFromBody := func(t *testing.T, rec *httptest.ResponseRecorder) dispensationpkg.Item {
		t.Helper()
		var body struct {
			Dispensation dispensationpkg.Item `json:"dispensation"`
		}
		decode(t, rec, &body)
		return body.Dispensation
	}

	t.Run("authentication and read rbac", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous list status = %d", rec.Code)
		}
		// The student role reaches no staff capability group.
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations", nil, data.studentToken); rec.Code != http.StatusForbidden {
			t.Fatalf("student list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations", nil, data.principalToken); rec.Code != http.StatusOK {
			t.Fatalf("principal list status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	var createdID string
	t.Run("staff create (json)", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "sick", "reason": "demam tiga hari",
			"start_date": data.today,
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("admin create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		item := itemFromBody(t, rec)
		if item.Status != "pending" || item.StudentID != data.studentA || item.StudentName != "Siswa Kelas A" {
			t.Fatalf("created item = %+v", item)
		}
		if item.ClassID != data.classA || item.Type != "sick" || item.StartDate != data.today {
			t.Fatalf("class snapshot / dates wrong: %+v", item)
		}
		if item.DocumentAvailable || item.VerifiedBy != nil || item.VerifiedAt != nil || item.SubmittedBy == "" {
			t.Fatalf("create defaults wrong: %+v", item)
		}
		createdID = item.ID

		// Same student, same type, same start date: duplicate blocked (409).
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "sick", "reason": "sama", "start_date": data.today,
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// staff_tu and duty_teacher may create; principal may not.
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentB, "type": "permit", "reason": "acara keluarga", "start_date": data.today,
		}, data.staffTU); rec.Code != http.StatusCreated {
			t.Fatalf("staff_tu create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "late", "reason": "banjir", "start_date": data.today,
		}, data.dutyToken); rec.Code != http.StatusCreated {
			t.Fatalf("duty_teacher create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "other", "reason": "kepala sekolah", "start_date": shiftDate(data.today, 3),
		}, data.principalToken); rec.Code != http.StatusForbidden {
			t.Fatalf("principal create status = %d", rec.Code)
		}
		// A teacher may only create for students of their own homeroom class
		// (server-side scope, not just the route group).
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "other", "reason": "bukan kelas saya", "start_date": shiftDate(data.today, 4),
		}, data.otherTeach); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign-class teacher create status = %d, want 403", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentB, "type": "other", "reason": "bukan kelas saya", "start_date": shiftDate(data.today, 4),
		}, data.homeroomToken); rec.Code != http.StatusForbidden {
			t.Fatalf("class-A homeroom create for class-B student status = %d, want 403", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "other", "reason": "dari wali kelas", "start_date": shiftDate(data.today, 5),
		}, data.homeroomToken); rec.Code != http.StatusCreated {
			t.Fatalf("own-class teacher create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// Validation and existence errors.
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "izin", "reason": "x", "start_date": data.today,
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid type status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispendations", nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("typo path status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": "not-a-uuid", "type": "sick", "reason": "x", "start_date": data.today,
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad student id status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": "00000000-0000-4000-8000-000000000000", "type": "sick", "reason": "x", "start_date": data.today,
		}, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown student status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "other", "reason": "  ", "start_date": data.today,
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("blank reason status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations", map[string]any{
			"student_id": data.studentA, "type": "other", "reason": "acara", "start_date": data.today, "end_date": "2020-01-01",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("end before start status = %d", rec.Code)
		}
	})

	var docItemID string
	t.Run("multipart create with document", func(t *testing.T) {
		pdf := append([]byte("%PDF-1.4 fake minimal pdf"), bytes.Repeat([]byte("x"), 512)...)
		rec := env.multipartRequest(t, "/api/v1/dispensations", map[string]string{
			"student_id": data.studentA, "type": "other", "reason": "lomba di luar",
			"start_date": data.today, "end_date": data.today,
		}, "document", "surat-lomba.pdf", "application/pdf", pdf, data.homeroomToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("teacher multipart create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		item := itemFromBody(t, rec)
		if !item.DocumentAvailable || item.EndDate == nil || *item.EndDate != data.today {
			t.Fatalf("multipart item = %+v", item)
		}
		docItemID = item.ID

		// Rejected documents.
		rec = env.multipartRequest(t, "/api/v1/dispensations", map[string]string{
			"student_id": data.studentA, "type": "other", "reason": "bukan pdf",
			"start_date": shiftDate(data.today, 2),
		}, "document", "catatan.txt", "text/plain", []byte("hello world"), data.adminToken)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("txt upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// Oversize: the service limit is pinned to 1 KiB via IFTER_IMPORTS_MAX_FILE_SIZE.
		oversize := append([]byte("%PDF-1.4 "), bytes.Repeat([]byte("x"), 3000)...)
		rec = env.multipartRequest(t, "/api/v1/dispensations", map[string]string{
			"student_id": data.studentA, "type": "other", "reason": "kebesaran",
			"start_date": shiftDate(data.today, 1),
		}, "document", "besar.pdf", "application/pdf", oversize, data.adminToken)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("oversize upload status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("document download", func(t *testing.T) {
		rec := env.rawRequest(http.MethodGet, "/api/v1/dispensations/"+docItemID+"/document", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("download status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("nosniff header = %q", got)
		}
		if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, `attachment; filename="surat-lomba.pdf"`) {
			t.Fatalf("content disposition = %q", got)
		}
		if got := rec.Header().Get("Content-Type"); got != "application/pdf" {
			t.Fatalf("download content type = %q", got)
		}
		if !strings.HasPrefix(rec.Body.String(), "%PDF-1.4") {
			t.Fatalf("download body = %q", rec.Body.String())
		}

		// Teacher of the other class may not download.
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations/"+docItemID+"/document", nil, data.otherTeach); rec.Code != http.StatusForbidden {
			t.Fatalf("other-class teacher download status = %d", rec.Code)
		}
		// No document attached: 404.
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations/"+createdID+"/document", nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("no-document download status = %d", rec.Code)
		}
		// Files are stored under random keys inside the private dir.
		entries, err := os.ReadDir(env.dir)
		if err != nil || len(entries) == 0 {
			t.Fatalf("storage dir entries = %v, err = %v", entries, err)
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), "surat-lomba") {
				t.Fatalf("document stored under client filename: %s", entry.Name())
			}
		}
	})

	t.Run("list scope and filters", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/dispensations?student_id="+data.studentA, nil, data.staffTU)
		if rec.Code != http.StatusOK {
			t.Fatalf("staff_tu list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var all dispensationpkg.ListResult
		decode(t, rec, &all)
		if all.Total < 3 || len(all.Data) < 3 {
			t.Fatalf("student A items = %+v", all)
		}
		for _, item := range all.Data {
			if item.StudentID != data.studentA {
				t.Fatalf("student_id filter leaked: %+v", item)
			}
		}
		if all.Limit != 50 || all.Offset != 0 {
			t.Fatalf("default page = %d/%d", all.Limit, all.Offset)
		}

		// Homeroom teacher only sees class A; the other teacher only class B.
		rec = env.request(t, http.MethodGet, "/api/v1/dispensations", nil, data.homeroomToken)
		var hr dispensationpkg.ListResult
		decode(t, rec, &hr)
		for _, item := range hr.Data {
			if item.ClassID != data.classA {
				t.Fatalf("homeroom teacher saw foreign class: %+v", item)
			}
		}
		if hr.Total < 3 {
			t.Fatalf("homeroom teacher total = %d, want >= 3", hr.Total)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/dispensations", nil, data.otherTeach)
		var other dispensationpkg.ListResult
		decode(t, rec, &other)
		if other.Total != 1 || other.Data[0].ClassID != data.classB {
			t.Fatalf("other-class teacher scope = %+v", other)
		}

		// status/type/from/to filters and pagination.
		rec = env.request(t, http.MethodGet, "/api/v1/dispensations?status=pending&type=sick", nil, data.adminToken)
		var pending dispensationpkg.ListResult
		decode(t, rec, &pending)
		if pending.Total != 1 || pending.Data[0].Type != "sick" {
			t.Fatalf("status+type filter = %+v", pending)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/dispensations?from="+data.today+"&to="+data.today, nil, data.adminToken)
		var ranged dispensationpkg.ListResult
		decode(t, rec, &ranged)
		if ranged.Total < 3 {
			t.Fatalf("date range total = %d", ranged.Total)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/dispensations?limit=2&offset=0", nil, data.adminToken)
		var paged dispensationpkg.ListResult
		decode(t, rec, &paged)
		if len(paged.Data) != 2 || paged.Limit != 2 || paged.Offset != 0 || paged.Total < 3 {
			t.Fatalf("paged = %+v", paged)
		}
		for _, bad := range []string{"?limit=500", "?offset=-1", "?status=verified", "?type=izin", "?from=nope", "?class_id=nope"} {
			if rec := env.request(t, http.MethodGet, "/api/v1/dispensations"+bad, nil, data.adminToken); rec.Code != http.StatusBadRequest {
				t.Fatalf("bad query %s status = %d", bad, rec.Code)
			}
		}

		// Single fetch: own-class teacher 200, foreign-class teacher 403, principal 200.
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations/"+createdID, nil, data.homeroomToken); rec.Code != http.StatusOK {
			t.Fatalf("homeroom get status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations/"+createdID, nil, data.otherTeach); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign-class get status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations/"+createdID, nil, data.principalToken); rec.Code != http.StatusOK {
			t.Fatalf("principal get status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations/not-a-uuid", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad id status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/dispensations/00000000-0000-4000-8000-000000000000", nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown id status = %d", rec.Code)
		}
	})

	t.Run("verify decisions", func(t *testing.T) {
		// staff_tu and principal are excluded from the ops group.
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations/"+createdID+"/verify",
			map[string]any{"status": "approved"}, data.staffTU); rec.Code != http.StatusForbidden {
			t.Fatalf("staff_tu verify status = %d", rec.Code)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations/"+createdID+"/verify",
			map[string]any{"status": "approved"}, data.principalToken); rec.Code != http.StatusForbidden {
			t.Fatalf("principal verify status = %d", rec.Code)
		}
		// A teacher who is not the homeroom teacher of that class: server-side 403.
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations/"+createdID+"/verify",
			map[string]any{"status": "approved", "note": "bukan kelas saya"}, data.otherTeach); rec.Code != http.StatusForbidden {
			t.Fatalf("foreign-class teacher verify status = %d", rec.Code)
		}
		// Invalid decision values.
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations/"+createdID+"/verify",
			map[string]any{"status": "pending"}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("pending decision status = %d", rec.Code)
		}
		// The homeroom teacher approves own-class item.
		rec := env.request(t, http.MethodPost, "/api/v1/dispensations/"+createdID+"/verify",
			map[string]any{"status": "approved", "note": "dibaca dari surat"}, data.homeroomToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("homeroom verify status = %d, body = %s", rec.Code, rec.Body.String())
		}
		item := itemFromBody(t, rec)
		if item.Status != "approved" || item.VerifiedBy == nil || item.VerifiedAt == nil {
			t.Fatalf("verified item = %+v", item)
		}
		if item.Note == nil || *item.Note != "dibaca dari surat" {
			t.Fatalf("verification note = %+v", item.Note)
		}
		// Double verify is a conflict.
		if rec := env.request(t, http.MethodPost, "/api/v1/dispensations/"+createdID+"/verify",
			map[string]any{"status": "rejected"}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("second verify status = %d", rec.Code)
		}
		// duty_teacher rejects school-wide.
		rec = env.request(t, http.MethodPost, "/api/v1/dispensations/"+docItemID+"/verify",
			map[string]any{"status": "rejected", "note": "tidak ada rekomendasi"}, data.dutyToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("duty verify status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := itemFromBody(t, rec); got.Status != "rejected" {
			t.Fatalf("duty decision = %+v", got)
		}
		// Verify writes audit rows.
		var auditCount int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM audit_logs WHERE action = 'dispensation.verify' AND entity_type = 'dispensation'",
		).Scan(&auditCount); err != nil {
			t.Fatalf("count dispensation audit: %v", err)
		}
		if auditCount < 2 {
			t.Fatalf("dispensation.verify audit rows = %d, want >= 2", auditCount)
		}
	})

	t.Run("my dispensations require the student account link", func(t *testing.T) {
		// students.user_id is added by migration 000015 (student-accounts).
		// Until that link exists the /my surface must answer 403, and only
		// the student role may reach it at all.
		if rec := env.request(t, http.MethodGet, "/api/v1/my/dispensations", nil, data.studentToken); rec.Code != http.StatusForbidden {
			t.Fatalf("student my-list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/my/dispensations", nil, data.adminToken); rec.Code != http.StatusForbidden {
			t.Fatalf("admin my-list status = %d", rec.Code)
		}
		rec := env.multipartRequest(t, "/api/v1/my/dispensations", map[string]string{
			"type": "sick", "reason": "sakit sendiri", "start_date": shiftDate(data.today, 5),
		}, "", "", "", nil, data.studentToken)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("student my-create status = %d, body = %s", rec.Code, rec.Body.String())
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
	name := fmt.Sprintf("ifter_dispensations_test_%d", time.Now().UnixNano())
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
	// Pin the shared imports storage settings so the oversize test stays cheap
	// and the documents land in a temp dir. Read before httptransport.New()
	// resolves FeatureDeps via dispensations.ConfigFromEnvironment().
	dir := t.TempDir()
	t.Setenv("IFTER_IMPORTS_DIR", dir)
	t.Setenv("IFTER_IMPORTS_MAX_FILE_SIZE", "1024")
	tokens := authpkg.NewTokenManager("test-secret-at-least-32-characters-long", "test-issuer", 15*time.Minute)
	authService := authpkg.NewService(pool, tokens, authpkg.ServiceConfig{
		RefreshTokenTTL: 24 * time.Hour,
		InviteTokenTTL:  72 * time.Hour,
		ResetTokenTTL:   time.Hour,
		BcryptCost:      bcrypt.MinCost,
	}, log)
	// The pgx pool doubles as readiness; New() wires the dispensation service
	// from it (see router.go FeatureDeps).
	router := httptransport.New(log, pool, authService, nil, nil, nil, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router, dir: dir}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	adminID := e.insertUser(t, "disp-admin", "disp-admin@example.com", "disp-admin-password", "admin")
	tuID := e.insertUser(t, "disp-tu", "disp-tu@example.com", "disp-tu-password", "staff_tu")
	hrUserID := e.insertUser(t, "disp-hr", "disp-hr@example.com", "disp-hr-password", "teacher")
	otherUserID := e.insertUser(t, "disp-other", "disp-other@example.com", "disp-other-password", "teacher")
	dutyID := e.insertUser(t, "disp-duty", "disp-duty@example.com", "disp-duty-password", "duty_teacher")
	principalID := e.insertUser(t, "disp-principal", "disp-principal@example.com", "disp-principal-password", "principal")
	studentUserID := e.insertUser(t, "disp-student", "disp-student@example.com", "disp-student-password", "student")
	_, _, _, _, _, _ = adminID, tuID, dutyID, principalID, studentUserID, otherUserID

	yearID := e.insert(t,
		`INSERT INTO academic_years (name, start_date, end_date, is_active)
		 VALUES ($1, $2, $3, true) RETURNING id::text`,
		fmt.Sprintf("AY-DISP-%d", time.Now().UnixNano()),
		now.AddDate(0, 0, -120).Format("2006-01-02"),
		now.AddDate(0, 0, 240).Format("2006-01-02"),
	)
	classA := e.insert(t,
		`INSERT INTO classes (academic_year_id, name, grade_level) VALUES ($1, '7A', 7) RETURNING id::text`, yearID)
	classB := e.insert(t,
		`INSERT INTO classes (academic_year_id, name, grade_level) VALUES ($1, '7B', 7) RETURNING id::text`, yearID)
	studentA := e.insertStudent(t, "DISP-NIS-A", "Siswa Kelas A")
	studentB := e.insertStudent(t, "DISP-NIS-B", "Siswa Kelas B")
	enrollStart := now.AddDate(0, 0, -30).Format("2006-01-02")
	e.insertStudentEnrollment(t, studentA, classA, enrollStart)
	e.insertStudentEnrollment(t, studentB, classB, enrollStart)

	teacherA := e.insertTeacher(t, hrUserID, "Guru Wali A")
	teacherB := e.insertTeacher(t, otherUserID, "Guru Wali B")
	e.insertHomeroom(t, teacherA, classA, enrollStart)
	e.insertHomeroom(t, teacherB, classB, enrollStart)

	return testData{
		adminToken:     e.login(t, "disp-admin", "disp-admin-password"),
		staffTU:        e.login(t, "disp-tu", "disp-tu-password"),
		homeroomToken:  e.login(t, "disp-hr", "disp-hr-password"),
		otherTeach:     e.login(t, "disp-other", "disp-other-password"),
		dutyToken:      e.login(t, "disp-duty", "disp-duty-password"),
		principalToken: e.login(t, "disp-principal", "disp-principal-password"),
		studentToken:   e.login(t, "disp-student", "disp-student-password"),
		classA:         classA,
		classB:         classB,
		studentA:       studentA,
		studentB:       studentB,
		today:          today,
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

func (e apiEnv) insertStudentEnrollment(t *testing.T, studentID, classID, start string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		"INSERT INTO class_enrollments (student_id, class_id, start_date) VALUES ($1,$2,$3)",
		studentID, classID, start,
	); err != nil {
		t.Fatalf("insert enrollment: %v", err)
	}
}

func (e apiEnv) insertHomeroom(t *testing.T, teacherID, classID, start string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		"INSERT INTO homeroom_assignments (teacher_id, class_id, start_date) VALUES ($1,$2,$3)",
		teacherID, classID, start,
	); err != nil {
		t.Fatalf("insert homeroom: %v", err)
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
	return e.rawRequest(method, path, reader, token, func(req *http.Request) {
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
	})
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

// multipartRequest builds the create upload; filePart empty omits the file.
func (e apiEnv) multipartRequest(t *testing.T, path string, fields map[string]string,
	filePart, filename, contentType string, fileData []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatalf("write field %s: %v", name, err)
		}
	}
	if filePart != "" && fileData != nil {
		part, err := writer.CreatePart(map[string][]string{
			"Content-Disposition": {`form-data; name="` + filePart + `"; filename="` + filename + `"`},
			"Content-Type":        {contentType},
		})
		if err != nil {
			t.Fatalf("create typed part: %v", err)
		}
		if _, err := part.Write(fileData); err != nil {
			t.Fatalf("write file part: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
