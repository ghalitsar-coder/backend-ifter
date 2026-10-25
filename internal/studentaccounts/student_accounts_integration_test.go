package studentaccounts_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	studentaccountspkg "github.com/ghalitsar-coder/backend-ifter/internal/studentaccounts"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

type apiEnv struct {
	pool   *pgxpool.Pool
	router http.Handler
}

type testData struct {
	adminID      string
	adminToken   string
	staffTUToken string
	teacherToken string
	orphanToken  string // student role, never linked to a student row
	linkable     string // SAC-001, active and enrolled: the lifecycle subject
	second       string // SAC-002, active: username-conflict + explicit-username subject
	graduate     string // SAC-003, graduated: refused a login account
}

// studentPassword is the password the invited account sets through
// POST /auth/activate (auth requires at least 12 characters).
const studentPassword = "student-invite-pw"

func TestStudentAccountsAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	accountPath := func(studentID string) string { return "/api/v1/students/" + studentID + "/account" }

	t.Run("provisioning rbac", func(t *testing.T) {
		if rec := env.request(http.MethodPost, accountPath(data.linkable), nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous provision status = %d", rec.Code)
		}
		// Admin only (ADDENDUM §1/§6.7): staff_tu, teacher, and the student
		// role itself all stay out of the provisioning surface.
		for name, token := range map[string]string{
			"staff_tu": data.staffTUToken,
			"teacher":  data.teacherToken,
			"student":  data.orphanToken,
		} {
			if rec := env.request(http.MethodPost, accountPath(data.linkable), map[string]any{}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s provision status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
			if rec := env.request(http.MethodGet, accountPath(data.linkable), nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s get account status = %d", name, rec.Code)
			}
			if rec := env.request(http.MethodDelete, accountPath(data.linkable), nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s delete account status = %d", name, rec.Code)
			}
		}
	})

	t.Run("provision validation and errors", func(t *testing.T) {
		if rec := env.request(http.MethodPost, accountPath("not-a-uuid"), nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad student id status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(http.MethodPost, accountPath("00000000-0000-4000-8000-000000000000"), nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown student status = %d", rec.Code)
		}
		// A student who has left the school cannot use /my/*, so no login is minted.
		if rec := env.request(http.MethodPost, accountPath(data.graduate), nil, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("graduate status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// Whitespace inside an explicit username is rejected before any write.
		if rec := env.request(http.MethodPost, accountPath(data.second), map[string]any{
			"username": "ani widi",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad username status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.requestRawString(http.MethodPost, accountPath(data.second),
			`{"username":`, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed json status = %d", rec.Code)
		}
		var linked bool
		if err := env.pool.QueryRow(context.Background(),
			"SELECT user_id IS NOT NULL FROM students WHERE id = $1", data.second).Scan(&linked); err != nil {
			t.Fatalf("check second student: %v", err)
		}
		if linked {
			t.Fatal("a rejected provisioning attempt linked a login account")
		}
	})

	var provisioned struct {
		User       studentaccountspkg.UserAccount     `json:"user"`
		Credential studentaccountspkg.CredentialIssue `json:"credential"`
	}
	var secondInvite string // SAC-002's never-activated invite
	t.Run("admin provisions the first account", func(t *testing.T) {
		// Empty body: the username falls back to the student number.
		rec := env.request(http.MethodPost, accountPath(data.linkable), nil, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("provision status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("cache control = %q, want no-store", got)
		}
		decode(t, rec, &provisioned)
		if provisioned.User.ID == "" || provisioned.User.Username != "SAC-001" ||
			provisioned.User.FullName != "Ani Berlink" || provisioned.User.Status != "pending" ||
			len(provisioned.User.Roles) != 1 || provisioned.User.Roles[0] != "student" {
			t.Fatalf("provisioned user = %+v", provisioned.User)
		}
		if provisioned.User.LastLoginAt != nil {
			t.Fatalf("fresh account carries a last login: %+v", provisioned.User)
		}
		if provisioned.Credential.Token == "" || provisioned.Credential.Purpose != "invite" ||
			provisioned.Credential.ExpiresAt.Before(time.Now().Add(71*time.Hour)) {
			t.Fatalf("credential = %+v", provisioned.Credential)
		}
		// The token is not stored in clear text and no password exists yet.
		var passwordSet bool
		var cleared int
		if err := env.pool.QueryRow(context.Background(),
			`SELECT u.password_hash IS NOT NULL,
			        (SELECT count(*) FROM user_credential_tokens t WHERE t.user_id = u.id)
			 FROM users u WHERE u.id = $1`, provisioned.User.ID).Scan(&passwordSet, &cleared); err != nil {
			t.Fatalf("load provisioned user: %v", err)
		}
		if passwordSet {
			t.Fatal("provisioning wrote a password hash")
		}
		if cleared != 1 {
			t.Fatalf("credential tokens = %d, want 1 invite", cleared)
		}
		var userID string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT user_id::text FROM students WHERE id = $1", data.linkable).Scan(&userID); err != nil {
			t.Fatalf("student link after provisioning: %v", err)
		}
		if userID != provisioned.User.ID {
			t.Fatalf("students.user_id = %s, want %s", userID, provisioned.User.ID)
		}

		// Idempotency guard: the student is linked now, so 409 (never a second account).
		if rec := env.request(http.MethodPost, accountPath(data.linkable), nil, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("re-provision status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var accounts int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM users WHERE username = 'SAC-001'").Scan(&accounts); err != nil {
			t.Fatalf("count accounts: %v", err)
		}
		if accounts != 1 {
			t.Fatalf("accounts named SAC-001 = %d, want 1", accounts)
		}
	})

	t.Run("account view", func(t *testing.T) {
		rec := env.request(http.MethodGet, accountPath(data.linkable), nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("get account status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Account studentaccountspkg.Account `json:"account"`
		}
		decode(t, rec, &body)
		if body.Account.UserID == nil || *body.Account.UserID != provisioned.User.ID {
			t.Fatalf("account user id = %v, want %s", body.Account.UserID, provisioned.User.ID)
		}
		if body.Account.Username == nil || *body.Account.Username != "SAC-001" {
			t.Fatalf("account username = %v", body.Account.Username)
		}
		if body.Account.Status == nil || *body.Account.Status != "pending" {
			t.Fatalf("account status = %v, want pending", body.Account.Status)
		}
		if body.Account.LastLoginAt != nil {
			t.Fatalf("account last login = %v, want null", body.Account.LastLoginAt)
		}
		// Provisioning is admin-only, so staff_tu cannot even read the view.
		if rec := env.request(http.MethodGet, accountPath(data.second), nil, data.staffTUToken); rec.Code != http.StatusForbidden {
			t.Fatalf("staff_tu account view status = %d, want 403", rec.Code)
		}
		// A student without a login answers with nulls, not 404.
		rec = env.request(http.MethodGet, accountPath(data.second), nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("unlinked account view status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &body)
		if body.Account.UserID != nil || body.Account.Username != nil ||
			body.Account.Status != nil || body.Account.LastLoginAt != nil {
			t.Fatalf("unlinked account = %+v, want all null", body.Account)
		}
		if rec := env.request(http.MethodGet, accountPath("not-a-uuid"), nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad id account view status = %d", rec.Code)
		}
		if rec := env.request(http.MethodGet, accountPath("00000000-0000-4000-8000-000000000000"), nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown student account view status = %d", rec.Code)
		}
	})

	t.Run("the invited account activates and reaches its own portal", func(t *testing.T) {
		rec := env.request(http.MethodPost, "/api/v1/auth/activate", map[string]string{
			"token": provisioned.Credential.Token, "password": studentPassword,
		}, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("activate status = %d, body = %s", rec.Code, rec.Body.String())
		}
		token := env.login(t, "SAC-001", studentPassword)

		rec = env.request(http.MethodGet, "/api/v1/my/profile", nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("my profile status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var profile struct {
			Profile studentaccountspkg.Profile `json:"profile"`
		}
		decode(t, rec, &profile)
		if profile.Profile.StudentID != data.linkable || profile.Profile.StudentNumber != "SAC-001" ||
			profile.Profile.FullName != "Ani Berlink" {
			t.Fatalf("profile = %+v", profile.Profile)
		}
		if profile.Profile.ClassName == nil || *profile.Profile.ClassName != "12A" {
			t.Fatalf("profile class = %+v, want 12A", profile.Profile)
		}

		const window = "?from=2026-09-01&to=2026-09-30"
		rec = env.request(http.MethodGet, "/api/v1/my/attendance"+window, nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("my attendance status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var attendance studentaccountspkg.MyAttendanceList
		decode(t, rec, &attendance)
		if attendance.Total != 3 || attendance.Limit != 50 || attendance.Offset != 0 || len(attendance.Data) != 3 {
			t.Fatalf("attendance list = %+v", attendance)
		}
		// Newest first, exactly like GET /attendance/history.
		if attendance.Data[0].Date != "2026-09-09" || attendance.Data[0].Status != "absent" {
			t.Fatalf("first attendance row = %+v", attendance.Data[0])
		}
		if attendance.Data[2].Date != "2026-09-07" || attendance.Data[2].Status != "present" ||
			attendance.Data[2].CheckInTime == nil {
			t.Fatalf("last attendance row = %+v", attendance.Data[2])
		}
		for _, item := range attendance.Data {
			if item.ClassID == "" || item.ClassName != "12A" {
				t.Fatalf("row carries no class snapshot: %+v", item)
			}
		}
		// The package-spec alias answers the same view.
		if rec := env.request(http.MethodGet, "/api/v1/my/attendance-history"+window, nil, token); rec.Code != http.StatusOK {
			t.Fatalf("attendance-history alias status = %d", rec.Code)
		}
		// Pagination stays server-side.
		rec = env.request(http.MethodGet, "/api/v1/my/attendance?from=2026-09-01&to=2026-09-30&limit=2&offset=2", nil, token)
		decode(t, rec, &attendance)
		if attendance.Total != 3 || len(attendance.Data) != 1 || attendance.Limit != 2 || attendance.Offset != 2 {
			t.Fatalf("paged attendance = %+v", attendance)
		}

		rec = env.request(http.MethodGet, "/api/v1/my/attendance-summary"+window, nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("my attendance summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var summary studentaccountspkg.MyAttendanceSummary
		decode(t, rec, &summary)
		if summary.From != "2026-09-01" || summary.To != "2026-09-30" || len(summary.Data) != 1 {
			t.Fatalf("attendance summary window = %+v", summary)
		}
		day := summary.Data[0]
		if day.StudentID != data.linkable || day.StudentNumber != "SAC-001" || day.ClassName != "12A" {
			t.Fatalf("summary day identity = %+v", day)
		}
		if day.TotalDays != 3 || day.Present != 1 || day.Late != 0 || day.Absent != 1 ||
			day.Sick != 1 || day.Excused != 0 || day.Percentage != 33.33 {
			t.Fatalf("summary day counts = %+v", day)
		}
		if summary.Totals.Students != 1 || summary.Totals.TotalDays != 3 ||
			summary.Totals.Present != 1 || summary.Totals.Late != 0 || summary.Totals.Absent != 1 ||
			summary.Totals.Sick != 1 || summary.Totals.Percentage != 33.33 {
			t.Fatalf("summary totals = %+v", summary.Totals)
		}

		rec = env.request(http.MethodGet, "/api/v1/my/discipline"+window, nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("my discipline status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var discipline studentaccountspkg.MyDisciplineList
		decode(t, rec, &discipline)
		if discipline.Total != 3 || len(discipline.Data) != 3 {
			t.Fatalf("discipline list = %+v", discipline)
		}
		if discipline.Data[0].CategoryName == "" || discipline.Data[0].Description == "" ||
			discipline.Data[0].Status == "" || discipline.Data[0].ID == "" ||
			discipline.Data[0].OccurredAt.IsZero() {
			t.Fatalf("discipline row = %+v", discipline.Data[0])
		}

		rec = env.request(http.MethodGet, "/api/v1/my/discipline-summary"+window, nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("my discipline summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var disciplineSummary studentaccountspkg.MyDisciplineSummary
		decode(t, rec, &disciplineSummary)
		if disciplineSummary.StudentID != data.linkable || disciplineSummary.StudentNumber != "SAC-001" {
			t.Fatalf("discipline summary subject = %+v", disciplineSummary)
		}
		// The cancelled record is excluded from the aggregate but stays in the list.
		if disciplineSummary.TotalRecords != 2 || disciplineSummary.TotalPoints != 12 {
			t.Fatalf("discipline totals = %+v, want 2 records / 12 points", disciplineSummary)
		}
		if len(disciplineSummary.Categories) != 1 || disciplineSummary.Categories[0].Count != 2 ||
			disciplineSummary.Categories[0].Points != 12 ||
			disciplineSummary.Categories[0].CategoryName != "SAC terlambat" {
			t.Fatalf("discipline categories = %+v", disciplineSummary.Categories)
		}
		if disciplineSummary.CurrentLevel == nil || disciplineSummary.CurrentLevel.Action != "Pemanggilan orang tua" {
			t.Fatalf("current level = %+v, want the 10-19 tier", disciplineSummary.CurrentLevel)
		}
		if disciplineSummary.CurrentLevel.Level == nil || *disciplineSummary.CurrentLevel.Level != "sedang" {
			t.Fatalf("current level name = %+v", disciplineSummary.CurrentLevel)
		}

		// Query validation mirrors GET /attendance/history.
		for _, path := range []string{
			"/api/v1/my/attendance",
			"/api/v1/my/attendance?from=2026-09-01",
			"/api/v1/my/attendance?to=2026-09-30",
			"/api/v1/my/attendance?from=2026-09-30&to=2026-09-01",
			"/api/v1/my/attendance?from=2025-01-01&to=2026-12-31",
			"/api/v1/my/attendance?from=2026-09-01&to=2026-09-30&limit=0",
			"/api/v1/my/attendance?from=2026-09-01&to=2026-09-30&limit=101",
			"/api/v1/my/attendance?from=2026-09-01&to=2026-09-30&offset=-1",
			"/api/v1/my/attendance-summary?from=2026-09-01",
			"/api/v1/my/discipline?from=x&to=y",
			"/api/v1/my/discipline-summary?from=2026-09-01&to=2026-09-30&limit=abc",
		} {
			if rec := env.request(http.MethodGet, path, nil, token); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, body = %s", path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("a student role with no linked row is refused", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/my/profile",
			"/api/v1/my/attendance?from=2026-09-01&to=2026-09-30",
			"/api/v1/my/attendance-summary?from=2026-09-01&to=2026-09-30",
			"/api/v1/my/discipline?from=2026-09-01&to=2026-09-30",
			"/api/v1/my/discipline-summary?from=2026-09-01&to=2026-09-30",
		} {
			rec := env.request(http.MethodGet, path, nil, data.orphanToken)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s status = %d, body = %s", path, rec.Code, rec.Body.String())
			}
			var body struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			decode(t, rec, &body)
			if body.Code != "forbidden" || !strings.Contains(body.Message, "student record") {
				t.Fatalf("unlinked body = %+v", rec.Body.String())
			}
		}
		// Staff roles never reach the student portal at all.
		for _, path := range []string{"/api/v1/my/profile", "/api/v1/my/attendance?from=2026-09-01&to=2026-09-30"} {
			if rec := env.request(http.MethodGet, path, nil, data.adminToken); rec.Code != http.StatusForbidden {
				t.Fatalf("%s as admin status = %d", path, rec.Code)
			}
			if rec := env.request(http.MethodGet, path, nil, ""); rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s anonymous status = %d", path, rec.Code)
			}
		}
		// No client-supplied student id can widen the portal either: an unknown
		// query parameter is ignored, the id still comes from the token.
		token := env.login(t, "SAC-001", studentPassword)
		rec := env.request(http.MethodGet,
			"/api/v1/my/discipline?from=2026-09-01&to=2026-09-30&student_id="+data.graduate, nil, token)
		if rec.Code != http.StatusOK {
			t.Fatalf("cross-student attempt status = %d", rec.Code)
		}
		var discipline studentaccountspkg.MyDisciplineList
		decode(t, rec, &discipline)
		if discipline.Total != 3 {
			t.Fatalf("cross-student attempt total = %d, want the caller's own 3", discipline.Total)
		}
	})

	t.Run("provisioning with an explicit username is admin-only", func(t *testing.T) {
		payload := map[string]any{"username": "sac002.invited", "email": "budii@example.com"}
		if rec := env.request(http.MethodPost, accountPath(data.second), payload, data.staffTUToken); rec.Code != http.StatusForbidden {
			t.Fatalf("staff_tu provision status = %d, want 403, body = %s", rec.Code, rec.Body.String())
		}
		rec := env.request(http.MethodPost, accountPath(data.second), payload, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("admin provision status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var created struct {
			User       studentaccountspkg.UserAccount     `json:"user"`
			Credential studentaccountspkg.CredentialIssue `json:"credential"`
		}
		decode(t, rec, &created)
		if created.User.Username != "sac002.invited" || created.User.Email != "budii@example.com" ||
			created.User.FullName != "Budi Berlink" || created.User.Status != "pending" {
			t.Fatalf("admin provisioned user = %+v", created.User)
		}
		secondInvite = created.Credential.Token
		// A username that already exists collides on the users table (409) and
		// leaves the student unlinked.
		third := env.insert(t,
			`INSERT INTO students (student_number, full_name, admission_year, status)
			 VALUES ('SAC-004', 'Citra Baru', 2026, 'active') RETURNING id::text`)
		if rec := env.request(http.MethodPost, accountPath(third), map[string]any{
			"username": "SAC-001",
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate username status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var linked bool
		if err := env.pool.QueryRow(context.Background(),
			"SELECT user_id IS NOT NULL FROM students WHERE id = $1", third).Scan(&linked); err != nil {
			t.Fatalf("check third student: %v", err)
		}
		if linked {
			t.Fatal("a failed provisioning attempt linked a login account")
		}
	})

	t.Run("writes are audited", func(t *testing.T) {
		counts := func(action string) int {
			var n int
			if err := env.pool.QueryRow(context.Background(),
				`SELECT count(*) FROM audit_logs WHERE action = $1 AND entity_type = 'student'`, action,
			).Scan(&n); err != nil {
				t.Fatalf("count %s audit: %v", action, err)
			}
			return n
		}
		if n := counts("student_account.create"); n != 2 {
			t.Fatalf("create audit rows = %d, want 2", n)
		}
		if n := counts("student_account.delete"); n != 0 {
			t.Fatalf("delete audit rows = %d before any deletion, want 0", n)
		}
		// The after snapshot carries the link, never the invite token.
		var snapshots int
		if err := env.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_logs
			 WHERE action = 'student_account.create' AND entity_type = 'student'
			   AND after_data::text LIKE '%sac002.invited%'`,
		).Scan(&snapshots); err != nil {
			t.Fatalf("load audit snapshot: %v", err)
		}
		if snapshots != 1 {
			t.Fatalf("create snapshots carrying the new username = %d, want 1", snapshots)
		}
		var leaked int
		if err := env.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_logs
			 WHERE before_data::text LIKE $1 OR after_data::text LIKE $1`,
			"%"+provisioned.Credential.Token+"%",
		).Scan(&leaked); err != nil {
			t.Fatalf("count token leaks: %v", err)
		}
		if leaked != 0 {
			t.Fatal("an audit snapshot leaked an invite token")
		}
	})

	t.Run("deactivate withdraws the login and the portal", func(t *testing.T) {
		studentToken := env.login(t, "SAC-001", studentPassword)
		if rec := env.request(http.MethodGet, "/api/v1/my/profile", nil, studentToken); rec.Code != http.StatusOK {
			t.Fatalf("profile before deactivation status = %d", rec.Code)
		}
		if rec := env.request(http.MethodDelete, accountPath(data.linkable), nil, data.staffTUToken); rec.Code != http.StatusForbidden {
			t.Fatalf("staff_tu deactivate status = %d, want 403", rec.Code)
		}
		if rec := env.request(http.MethodDelete, accountPath(data.linkable), nil, data.adminToken); rec.Code != http.StatusNoContent {
			t.Fatalf("deactivate status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// Link cleared, sessions revoked, the account deactivated and the
		// student's history preserved.
		var linked bool
		if err := env.pool.QueryRow(context.Background(),
			"SELECT user_id IS NOT NULL FROM students WHERE id = $1", data.linkable).Scan(&linked); err != nil {
			t.Fatalf("check link after deactivation: %v", err)
		}
		if linked {
			t.Fatal("the student is still linked to a login account")
		}
		var liveSessions int
		if err := env.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL`,
			provisioned.User.ID).Scan(&liveSessions); err != nil {
			t.Fatalf("count live sessions: %v", err)
		}
		if liveSessions != 0 {
			t.Fatalf("live refresh tokens = %d, want 0", liveSessions)
		}
		var status string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT status FROM users WHERE id = $1", provisioned.User.ID).Scan(&status); err != nil {
			t.Fatalf("load deactivated user: %v", err)
		}
		if status != "inactive" {
			t.Fatalf("user status = %q, want inactive", status)
		}
		var history int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM attendance_records WHERE student_id = $1", data.linkable).Scan(&history); err != nil {
			t.Fatalf("count preserved history: %v", err)
		}
		if history != 3 {
			t.Fatalf("attendance rows = %d, want the preserved 3", history)
		}
		// The withdrawn login cannot use the portal any more: its account is
		// inactive, so Authenticate refuses the still-unexpired access token
		// outright (401). A student whose link alone disappears gets 403, which
		// the unlinked-role case above covers.
		if rec := env.request(http.MethodGet, "/api/v1/my/profile", nil, studentToken); rec.Code != http.StatusUnauthorized {
			t.Fatalf("profile after deactivation status = %d, body = %s", rec.Code, rec.Body.String())
		}
		// The account view falls back to nulls.
		rec := env.request(http.MethodGet, accountPath(data.linkable), nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("account view after deactivation status = %d", rec.Code)
		}
		var body struct {
			Account studentaccountspkg.Account `json:"account"`
		}
		decode(t, rec, &body)
		if body.Account.UserID != nil {
			t.Fatalf("account view after deactivation = %+v", body.Account)
		}
		// Deleting twice is a 404 (nothing linked any more), and the audit trail
		// recorded exactly one withdrawal.
		if rec := env.request(http.MethodDelete, accountPath(data.linkable), nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("re-deactivate status = %d", rec.Code)
		}
		if rec := env.request(http.MethodDelete, accountPath("not-a-uuid"), nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad id deactivate status = %d", rec.Code)
		}
		var deletes int
		if err := env.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_logs WHERE action = 'student_account.delete' AND entity_type = 'student'`,
		).Scan(&deletes); err != nil {
			t.Fatalf("count delete audits: %v", err)
		}
		if deletes != 1 {
			t.Fatalf("delete audit rows = %d, want 1", deletes)
		}
		// Re-provisioning works again from a clean link; the withdrawn username
		// stays taken (the deactivated account is preserved, not renamed), so the
		// admin supplies a new one.
		rec = env.request(http.MethodPost, accountPath(data.linkable), map[string]any{
			"username": "sac001.baru",
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("re-provision status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var again struct {
			User       studentaccountspkg.UserAccount     `json:"user"`
			Credential studentaccountspkg.CredentialIssue `json:"credential"`
		}
		decode(t, rec, &again)
		if again.User.ID == provisioned.User.ID {
			t.Fatal("re-provisioning reused the withdrawn account")
		}
		// The withdrawn account's used invite can never come back.
		if rec := env.request(http.MethodPost, "/api/v1/auth/activate", map[string]string{
			"token": provisioned.Credential.Token, "password": studentPassword,
		}, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("replayed invite status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("deactivating a never-activated invite keeps the users constraint", func(t *testing.T) {
		// SAC-002 is still 'pending' without a password hash: the withdrawal may
		// not force it to 'inactive' (000002 CHECK), it just drops the invite and
		// the link.
		if rec := env.request(http.MethodDelete, accountPath(data.second), nil, data.adminToken); rec.Code != http.StatusNoContent {
			t.Fatalf("deactivate pending status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var status string
		var invites int
		if err := env.pool.QueryRow(context.Background(),
			`SELECT u.status,
			        (SELECT count(*) FROM user_credential_tokens t
			          WHERE t.user_id = u.id AND t.used_at IS NULL)
			 FROM users u WHERE u.username = 'sac002.invited'`,
		).Scan(&status, &invites); err != nil {
			t.Fatalf("load pending user: %v", err)
		}
		if status != "pending" {
			t.Fatalf("pending user status = %q, want pending", status)
		}
		if invites != 0 {
			t.Fatalf("unused invite tokens = %d, want 0", invites)
		}
		// The withdrawn invite can no longer set a password.
		if rec := env.request(http.MethodPost, "/api/v1/auth/activate", map[string]string{
			"token": secondInvite, "password": studentPassword,
		}, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("revoked invite replay status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var linked bool
		if err := env.pool.QueryRow(context.Background(),
			"SELECT user_id IS NOT NULL FROM students WHERE id = $1", data.second).Scan(&linked); err != nil {
			t.Fatalf("check second student: %v", err)
		}
		if linked {
			t.Fatal("second student still linked after withdrawal")
		}
	})

	t.Run("concurrent provisioning links exactly once", func(t *testing.T) {
		// Two admins race on the same unlinked student with different usernames,
		// so the link itself (not the users.username constraint) decides the
		// winner: the loser gets 409 and its fresh invite is revoked.
		fresh := env.insert(t,
			`INSERT INTO students (student_number, full_name, admission_year, status)
			 VALUES ('SAC-005', 'Dewi Berlink', 2026, 'active') RETURNING id::text`)
		type outcome struct {
			status int
			body   string
		}
		results := make(chan outcome, 2)
		for _, username := range []string{"sac005-a", "sac005-b"} {
			go func(username string) {
				payload := `{"username":"` + username + `"}`
				rec := env.requestRawString(http.MethodPost, accountPath(fresh), payload, data.adminToken)
				results <- outcome{status: rec.Code, body: rec.Body.String()}
			}(username)
		}
		var created, conflicts int
		for i := 0; i < 2; i++ {
			got := <-results
			switch got.status {
			case http.StatusCreated:
				created++
			case http.StatusConflict:
				conflicts++
			default:
				t.Fatalf("racing provision status = %d, body = %s", got.status, got.body)
			}
		}
		if created != 1 || conflicts != 1 {
			t.Fatalf("race outcome created=%d conflicts=%d, want 1 and 1", created, conflicts)
		}
		var winner string
		var links int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(user_id), max(user_id::text) FROM students WHERE id = $1", fresh).Scan(&links, &winner); err != nil {
			t.Fatalf("load racing link: %v", err)
		}
		if links != 1 || winner == "" {
			t.Fatalf("student carries %d links (winner %q), want exactly 1", links, winner)
		}
		// If both requests reached the account creation step, the loser's fresh
		// invite must have been revoked as the compensation for the lost race;
		// if it lost the pre-check instead, no second account exists at all.
		var loser string
		loserTokens := -1
		err := env.pool.QueryRow(context.Background(),
			`SELECT u.id::text,
			        (SELECT count(*) FROM user_credential_tokens t WHERE t.user_id = u.id)
			 FROM users u WHERE u.username LIKE 'sac005-%' AND u.id::text <> $1`,
			winner).Scan(&loser, &loserTokens)
		switch {
		case errors.Is(err, pgx.ErrNoRows): // lost before creating an account
		case err != nil:
			t.Fatalf("look up the losing account: %v", err)
		case loserTokens != 0:
			t.Fatalf("losing account %s keeps %d credential tokens, want 0", loser, loserTokens)
		}
		if rec := env.request(http.MethodGet, accountPath(fresh), nil, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("account view after the race status = %d", rec.Code)
		} else if body := rec.Body.String(); !strings.Contains(body, `"username":"sac005-`) {
			t.Fatalf("account view after the race = %s", body)
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
	name := fmt.Sprintf("ifter_studentaccounts_test_%d", time.Now().UnixNano())
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
	// The pgx pool doubles as readiness, which is also the wiring seam:
	// router.New builds the student-accounts service on this pool with the
	// composition-root auth service as its provisioner.
	router := httptransport.New(log, pool, authService, nil, nil, nil, nil, nil, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router}
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	adminID := e.insertUser(t, "sacc-admin", "sacc-admin@example.com", "sacc-admin-pw", "admin")
	e.insertUser(t, "sacc-staff", "sacc-staff@example.com", "sacc-staff-pw", "staff_tu")
	e.insertUser(t, "sacc-teacher", "sacc-teacher@example.com", "sacc-teacher-pw", "teacher")
	e.insertUser(t, "sacc-orphan", "sacc-orphan@example.com", "sacc-orphan-pw", "student")

	yearID := e.insert(t,
		`INSERT INTO academic_years (name, start_date, end_date, is_active)
		 VALUES ('2026/2027', '2026-07-01', '2027-06-30', true) RETURNING id::text`)
	classID := e.insert(t,
		`INSERT INTO classes (academic_year_id, name, grade_level, major)
		 VALUES ($1, '12A', 12, 'IPA') RETURNING id::text`, yearID)

	linkable := e.insert(t,
		`INSERT INTO students (student_number, full_name, admission_year, status)
		 VALUES ('SAC-001', 'Ani Berlink', 2026, 'active') RETURNING id::text`)
	second := e.insert(t,
		`INSERT INTO students (student_number, full_name, admission_year, status)
		 VALUES ('SAC-002', 'Budi Berlink', 2026, 'active') RETURNING id::text`)
	graduate := e.insert(t,
		`INSERT INTO students (student_number, full_name, admission_year, status)
		 VALUES ('SAC-003', 'Citra Lulus', 2026, 'graduated') RETURNING id::text`)
	enrollmentID := e.insert(t,
		`INSERT INTO class_enrollments (student_id, class_id, start_date)
		 VALUES ($1, $2, '2026-07-01') RETURNING id::text`, linkable, classID)

	// Three attendance days: present, sick, absent. The records go in first,
	// because a trigger refuses writes on an already closed day. 'sick' (not
	// 'late') keeps the auto-discipline trigger out of this fixture, so the
	// discipline expectations stay about the records seeded below.
	for _, day := range []struct{ date, status string }{
		{"2026-09-07", "present"},
		{"2026-09-08", "sick"},
		{"2026-09-09", "absent"},
	} {
		checkIn := any(nil)
		if day.status == "present" {
			checkIn = day.date + "T06:55:00+07:00"
		}
		if _, err := e.pool.Exec(context.Background(),
			`INSERT INTO attendance_records
			 (student_id, enrollment_id, attendance_date, status, check_in_at, recorded_by_user_id, updated_by_user_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $6)`,
			linkable, enrollmentID, day.date, day.status, checkIn, adminID,
		); err != nil {
			t.Fatalf("attendance %s: %v", day.date, err)
		}
	}
	// Closing the days is what makes them count in GET /attendance/summary.
	for _, day := range []string{"2026-09-07", "2026-09-08", "2026-09-09"} {
		if _, err := e.pool.Exec(context.Background(),
			`INSERT INTO attendance_day_closures (class_id, attendance_date, close_type, closed_by_user_id)
			 VALUES ($1, $2, 'manual', $3)`, classID, day, adminID,
		); err != nil {
			t.Fatalf("close %s: %v", day, err)
		}
	}

	categoryID := e.insert(t,
		`INSERT INTO discipline_categories (name, default_points) VALUES ('SAC terlambat', 6) RETURNING id::text`)
	e.insert(t,
		`INSERT INTO discipline_records
		 (student_id, enrollment_id, category_id, occurred_at, description, points, status, recorded_by_user_id)
		 VALUES ($1, $2, $3, '2026-09-07T07:20:00+07:00', 'Datang setelah bel', 6, 'new', $4) RETURNING id::text`,
		linkable, enrollmentID, categoryID, adminID)
	e.insert(t,
		`INSERT INTO discipline_records
		 (student_id, enrollment_id, category_id, occurred_at, description, points, status, recorded_by_user_id)
		 VALUES ($1, $2, $3, '2026-09-08T07:25:00+07:00', 'Terlambat lagi', 6, 'completed', $4) RETURNING id::text`,
		linkable, enrollmentID, categoryID, adminID)
	e.insert(t,
		`INSERT INTO discipline_records
		 (student_id, enrollment_id, category_id, occurred_at, description, points, status, recorded_by_user_id)
		 VALUES ($1, $2, $3, '2026-09-09T07:30:00+07:00', 'Sudah dibatalkan', 5, 'cancelled', $4) RETURNING id::text`,
		linkable, enrollmentID, categoryID, adminID)
	e.insert(t,
		`INSERT INTO sanction_rules (name, min_points, max_points, action, level, is_active)
		 VALUES ('SAC telat', 10, 19, 'Pemanggilan orang tua', 'sedang', true) RETURNING id::text`)

	return testData{
		adminID:      adminID,
		adminToken:   e.login(t, "sacc-admin", "sacc-admin-pw"),
		staffTUToken: e.login(t, "sacc-staff", "sacc-staff-pw"),
		teacherToken: e.login(t, "sacc-teacher", "sacc-teacher-pw"),
		orphanToken:  e.login(t, "sacc-orphan", "sacc-orphan-pw"),
		linkable:     linkable,
		second:       second,
		graduate:     graduate,
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
	rec := e.request(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": username, "password": password,
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login %s status = %d, body = %s", username, rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
	}
	decode(t, rec, &body)
	if body.AccessToken == "" {
		t.Fatalf("login %s returned no access token", username)
	}
	return body.AccessToken
}

func (e apiEnv) request(method, path string, body any, token string) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			panic(err)
		}
		reader = bytes.NewReader(payload)
	}
	return e.requestRaw(method, path, reader, token)
}

func (e apiEnv) requestRaw(method, path string, body io.Reader, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
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

// The malformed-json case needs a raw, invalid payload.
func (e apiEnv) requestRawString(method, path, payload, token string) *httptest.ResponseRecorder {
	return e.requestRaw(method, path, strings.NewReader(payload), token)
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
