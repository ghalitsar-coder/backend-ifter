package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

const (
	testSecret      = "test-secret-at-least-32-characters-long"
	testFrontendURL = "http://frontend.test"
)

type testEnv struct {
	pool    *pgxpool.Pool
	service *authpkg.Service
	router  http.Handler
}

func TestBootstrapAdminIsIdempotent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	admin := authpkg.BootstrapAdmin{
		Username: "bootstrap",
		Email:    "bootstrap@example.com",
		Password: "bootstrap-password",
		FullName: "Bootstrap Admin",
	}

	if err := env.service.EnsureBootstrapAdmin(ctx, admin); err != nil {
		t.Fatalf("EnsureBootstrapAdmin() error = %v", err)
	}
	if err := env.service.EnsureBootstrapAdmin(ctx, admin); err != nil {
		t.Fatalf("second EnsureBootstrapAdmin() error = %v", err)
	}
	if err := env.service.EnsureBootstrapAdmin(ctx, authpkg.BootstrapAdmin{
		Username: "second-admin",
		Password: "second-admin-password",
	}); err != nil {
		t.Fatalf("EnsureBootstrapAdmin() with existing admin error = %v", err)
	}

	assertCount(t, env.pool, "SELECT count(*) FROM users WHERE username = 'bootstrap'", 1)
	assertCount(t, env.pool, "SELECT count(*) FROM users WHERE username = 'second-admin'", 0)
	assertCount(t, env.pool, `SELECT count(*) FROM user_roles ur
		JOIN users u ON u.id = ur.user_id
		WHERE u.username = 'bootstrap' AND ur.role_code = 'admin'`, 1)
	assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'user.bootstrap_admin'", 1)

	session, err := env.service.Login(ctx, admin.Username, admin.Password, "bootstrap-login")
	if err != nil {
		t.Fatalf("Login() after bootstrap error = %v", err)
	}
	if !contains(session.User.Roles, "admin") {
		t.Fatalf("bootstrap roles = %#v, want admin", session.User.Roles)
	}
}

func TestAuthAPI(t *testing.T) {
	env := newTestEnv(t)
	teacher := env.insertUser(t, "teacher", "teacher@example.com", "active", "teacher-password", "teacher")
	env.insertUser(t, "inactive", "", "inactive", "inactive-password", "teacher")
	env.insertUser(t, "admin", "", "active", "admin-password", "admin")

	t.Run("login and me", func(t *testing.T) {
		login := env.login(t, "teacher", "teacher-password")
		if login.AccessToken == "" || login.TokenType != "Bearer" || login.ExpiresIn <= 0 {
			t.Fatalf("unexpected login response: %+v", login)
		}
		if login.User.ID != teacher || !contains(login.User.Roles, "teacher") {
			t.Fatalf("unexpected login user: %+v", login.User)
		}
		if login.Cookie.Name != "ifter_refresh" || login.Cookie.Value == "" {
			t.Fatalf("unexpected refresh cookie: %+v", login.Cookie)
		}
		if !login.Cookie.HttpOnly || login.Cookie.SameSite != http.SameSiteStrictMode || login.Cookie.Path != "/api/v1/auth" {
			t.Fatalf("refresh cookie security attributes: %+v", login.Cookie)
		}

		rec := env.request(t, http.MethodGet, "/api/v1/auth/me", nil, login.AccessToken, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("me status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body struct {
			User authpkg.User `json:"user"`
		}
		decode(t, rec, &body)
		if body.User.ID != teacher || !contains(body.User.Roles, "teacher") {
			t.Fatalf("me user = %+v", body.User)
		}
	})

	t.Run("invalid credentials and inactive account do not reveal existence", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"login": "teacher", "password": "wrong-password",
		}, "", nil, testFrontendURL)
		if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "invalid_credentials" {
			t.Fatalf("wrong password response = %d %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"login": "inactive", "password": "inactive-password",
		}, "", nil, testFrontendURL)
		if rec.Code != http.StatusUnauthorized || errorCode(t, rec) != "invalid_credentials" {
			t.Fatalf("inactive login response = %d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("rbac and admin user list", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/users", nil, "", nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated list status = %d", rec.Code)
		}

		teacherLogin := env.login(t, "teacher", "teacher-password")
		rec = env.request(t, http.MethodGet, "/api/v1/users", nil, teacherLogin.AccessToken, nil, "")
		if rec.Code != http.StatusForbidden || errorCode(t, rec) != "forbidden" {
			t.Fatalf("teacher list response = %d %s", rec.Code, rec.Body.String())
		}

		adminLogin := env.login(t, "admin", "admin-password")
		rec = env.request(t, http.MethodGet, "/api/v1/users?limit=10&offset=0", nil, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("admin list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list struct {
			Data []authpkg.UserSummary `json:"data"`
		}
		decode(t, rec, &list)
		if len(list.Data) == 0 {
			t.Fatal("admin list returned no users")
		}

		for _, path := range []string{"/api/v1/users?limit=101", "/api/v1/users?offset=-1", "/api/v1/users?status=bogus"} {
			rec = env.request(t, http.MethodGet, path, nil, adminLogin.AccessToken, nil, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, body = %s", path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("deactivated user loses access immediately", func(t *testing.T) {
		login := env.login(t, "teacher", "teacher-password")
		if _, err := env.pool.Exec(context.Background(),
			"UPDATE users SET status = 'inactive' WHERE id = $1", teacher,
		); err != nil {
			t.Fatalf("deactivate teacher: %v", err)
		}

		rec := env.request(t, http.MethodGet, "/api/v1/auth/me", nil, login.AccessToken, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("deactivated me status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", login.Cookie, testFrontendURL)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("deactivated refresh status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
}

func TestAccountManagementAPI(t *testing.T) {
	env := newTestEnv(t)
	env.insertUser(t, "account-admin", "account-admin@example.com", "active", "account-admin-password", "admin")
	adminLogin := env.login(t, "account-admin", "account-admin-password")

	t.Run("admin creates an invited teacher account", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/users", map[string]any{
			"username":  "new-teacher",
			"email":     "new-teacher@example.com",
			"full_name": "New Teacher",
			"roles":     []string{"teacher"},
		}, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create user status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var created struct {
			User       authpkg.UserAccount     `json:"user"`
			Credential authpkg.CredentialIssue `json:"credential"`
		}
		decode(t, rec, &created)
		if created.User.Status != "pending" || !contains(created.User.Roles, "teacher") {
			t.Fatalf("created user = %+v", created.User)
		}
		if created.Credential.Token == "" || created.Credential.Purpose != "invite" || created.Credential.ExpiresAt.IsZero() {
			t.Fatalf("created credential = %+v", created.Credential)
		}
		if created.Credential.ExpiresAt.Before(time.Now().Add(71 * time.Hour)) {
			t.Fatalf("invite expiry = %s, want about 72h", created.Credential.ExpiresAt)
		}

		var tokenHash string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT token_hash FROM user_credential_tokens WHERE user_id = $1 AND purpose = 'invite'", created.User.ID,
		).Scan(&tokenHash); err != nil {
			t.Fatalf("load invite token hash: %v", err)
		}
		if tokenHash != hashForTest(created.Credential.Token) {
			t.Fatal("stored token is not a hash of the returned token")
		}
		assertCount(t, env.pool, `SELECT count(*) FROM audit_logs
			WHERE action = 'user.create' AND entity_id = '`+created.User.ID+`'`, 1)
		assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'user.create' AND after_data::text LIKE '%new-teacher%'", 1)
		if err := env.request(t, http.MethodPost, "/api/v1/users", map[string]any{
			"username": "new-teacher", "full_name": "Duplicate", "roles": []string{"teacher"},
		}, adminLogin.AccessToken, nil, ""); err.Code != http.StatusConflict {
			t.Fatalf("duplicate username status = %d, body = %s", err.Code, err.Body.String())
		}

		rec = env.request(t, http.MethodGet, "/api/v1/users/"+created.User.ID, nil, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("get user status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var got struct {
			User authpkg.UserAccount `json:"user"`
		}
		decode(t, rec, &got)
		if got.User.ID != created.User.ID || !contains(got.User.Roles, "teacher") {
			t.Fatalf("get user = %+v", got.User)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/roles", nil, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list roles status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var roles struct {
			Data []authpkg.Role `json:"data"`
		}
		decode(t, rec, &roles)
		if len(roles.Data) != 7 {
			t.Fatalf("roles = %+v, want 7 seeded roles", roles.Data)
		}
	})

	created := func(t *testing.T, username string) (string, string) {
		rec := env.request(t, http.MethodPost, "/api/v1/users", map[string]any{
			"username": username, "full_name": "Invite Flow", "roles": []string{"teacher"},
		}, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("create invite-flow status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body struct {
			User       authpkg.UserAccount     `json:"user"`
			Credential authpkg.CredentialIssue `json:"credential"`
		}
		decode(t, rec, &body)
		return body.User.ID, body.Credential.Token
	}

	t.Run("invite activation enforces password policy and is one-time", func(t *testing.T) {
		userID, inviteToken := created(t, "invite-flow")

		rec := env.request(t, http.MethodPatch, "/api/v1/users/"+userID+"/status", map[string]string{
			"status": "active",
		}, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusConflict || errorCode(t, rec) != "user_not_ready" {
			t.Fatalf("activate pending status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/auth/activate", map[string]string{
			"token": inviteToken, "password": "short",
		}, "", nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("short password status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/auth/activate", map[string]string{
			"token": inviteToken, "password": "invite-password",
		}, "", nil, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("activate status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertCount(t, env.pool, "SELECT count(*) FROM users WHERE id = '"+userID+"' AND status = 'active' AND password_hash IS NOT NULL", 1)
		assertCount(t, env.pool, "SELECT count(*) FROM user_credential_tokens WHERE user_id = '"+userID+"' AND used_at IS NOT NULL", 1)
		assertCount(t, env.pool, `SELECT count(*) FROM audit_logs
			WHERE action = 'user.activate' AND entity_id = '`+userID+`'`, 1)

		login := env.login(t, "invite-flow", "invite-password")
		if !contains(login.User.Roles, "teacher") {
			t.Fatalf("activated user roles = %#v", login.User.Roles)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/auth/activate", map[string]string{
			"token": inviteToken, "password": "invite-password",
		}, "", nil, "")
		if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "invalid_token" {
			t.Fatalf("reused invite status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("roles and status updates", func(t *testing.T) {
		userID, inviteToken := created(t, "roles-flow")
		if rec := env.request(t, http.MethodPost, "/api/v1/auth/activate", map[string]string{
			"token": inviteToken, "password": "role-password",
		}, "", nil, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("activate role user status = %d, body = %s", rec.Code, rec.Body.String())
		}
		login := env.login(t, "roles-flow", "role-password")

		rec := env.request(t, http.MethodPut, "/api/v1/users/"+userID+"/roles", map[string]any{
			"roles": []string{"teacher", "principal"},
		}, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("update roles status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var updated struct {
			User authpkg.UserAccount `json:"user"`
		}
		decode(t, rec, &updated)
		if !contains(updated.User.Roles, "principal") {
			t.Fatalf("updated roles = %#v", updated.User.Roles)
		}
		assertCount(t, env.pool, `SELECT count(*) FROM audit_logs
			WHERE action = 'user.roles.update' AND entity_id = '`+userID+`'`, 1)

		rec = env.request(t, http.MethodPatch, "/api/v1/users/"+userID+"/status", map[string]string{
			"status": "inactive",
		}, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("deactivate status = %d, body = %s", rec.Code, rec.Body.String())
		}
		assertCount(t, env.pool, "SELECT count(*) FROM refresh_tokens WHERE user_id = '"+userID+"' AND revoked_at IS NULL", 0)
		if rec := env.request(t, http.MethodGet, "/api/v1/auth/me", nil, login.AccessToken, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("inactive me status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", login.Cookie, testFrontendURL); rec.Code != http.StatusUnauthorized {
			t.Fatalf("inactive refresh status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPatch, "/api/v1/users/"+userID+"/status", map[string]string{
			"status": "active",
		}, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("reactivate status = %d, body = %s", rec.Code, rec.Body.String())
		}

		rec = env.request(t, http.MethodPost, "/api/v1/users/"+userID+"/credential-reset", nil, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("issue reset status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var reset struct {
			Credential authpkg.CredentialIssue `json:"credential"`
		}
		decode(t, rec, &reset)
		if reset.Credential.Purpose != "reset" || reset.Credential.Token == "" {
			t.Fatalf("reset credential = %+v", reset.Credential)
		}
		if reset.Credential.ExpiresAt.After(time.Now().Add(2 * time.Hour)) {
			t.Fatalf("reset expiry = %s, want about 1h", reset.Credential.ExpiresAt)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/auth/activate", map[string]string{
			"token": reset.Credential.Token, "password": "new-role-password",
		}, "", nil, ""); rec.Code != http.StatusNoContent {
			t.Fatalf("consume reset status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/auth/me", nil, login.AccessToken, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("access token after password reset status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
			"login": "roles-flow", "password": "role-password",
		}, "", nil, testFrontendURL); rec.Code != http.StatusUnauthorized {
			t.Fatalf("old password after reset status = %d, body = %s", rec.Code, rec.Body.String())
		}
		env.login(t, "roles-flow", "new-role-password")
		assertCount(t, env.pool, `SELECT count(*) FROM audit_logs
			WHERE action = 'user.credential.reset' AND entity_id = '`+userID+`'`, 1)
		assertCount(t, env.pool, `SELECT count(*) FROM audit_logs
			WHERE action = 'user.password.reset' AND entity_id = '`+userID+`'`, 1)
	})

	t.Run("access control and invalid input", func(t *testing.T) {
		env.insertUser(t, "access-teacher", "", "active", "access-password", "teacher")
		teacherLogin := env.login(t, "access-teacher", "access-password")
		rec := env.request(t, http.MethodPost, "/api/v1/users", map[string]any{
			"username": "forbidden", "full_name": "Forbidden", "roles": []string{"teacher"},
		}, teacherLogin.AccessToken, nil, "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("teacher create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/roles", nil, teacherLogin.AccessToken, nil, "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("teacher roles status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodPost, "/api/v1/users", map[string]any{
			"username": "bad-role", "full_name": "Bad Role", "roles": []string{"does-not-exist"},
		}, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unknown role status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/users/not-a-uuid", nil, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid user id status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/users/00000000-0000-0000-0000-000000000000", nil, adminLogin.AccessToken, nil, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("missing user status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})
}

func TestLastAdminAndSystemAccountProtection(t *testing.T) {
	env := newTestEnv(t)
	adminID := env.insertUser(t, "sole-admin", "", "active", "sole-admin-password", "admin")
	adminLogin := env.login(t, "sole-admin", "sole-admin-password")

	if rec := env.request(t, http.MethodPatch, "/api/v1/users/"+adminID+"/status", map[string]string{
		"status": "inactive",
	}, adminLogin.AccessToken, nil, ""); rec.Code != http.StatusConflict || errorCode(t, rec) != "last_admin" {
		t.Fatalf("deactivate last admin status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := env.request(t, http.MethodPut, "/api/v1/users/"+adminID+"/roles", map[string]any{
		"roles": []string{"teacher"},
	}, adminLogin.AccessToken, nil, ""); rec.Code != http.StatusConflict || errorCode(t, rec) != "last_admin" {
		t.Fatalf("demote last admin status = %d, body = %s", rec.Code, rec.Body.String())
	}

	secondID := env.insertUser(t, "second-admin", "", "active", "second-admin-password", "admin")
	if rec := env.request(t, http.MethodPut, "/api/v1/users/"+adminID+"/roles", map[string]any{
		"roles": []string{"teacher"},
	}, adminLogin.AccessToken, nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("demote first admin status = %d, body = %s", rec.Code, rec.Body.String())
	}
	secondLogin := env.login(t, "second-admin", "second-admin-password")
	if rec := env.request(t, http.MethodPut, "/api/v1/users/"+secondID+"/roles", map[string]any{
		"roles": []string{"teacher"},
	}, secondLogin.AccessToken, nil, ""); rec.Code != http.StatusConflict || errorCode(t, rec) != "last_admin" {
		t.Fatalf("demote last remaining admin status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := env.request(t, http.MethodPatch, "/api/v1/users/"+secondID+"/status", map[string]string{
		"status": "inactive",
	}, secondLogin.AccessToken, nil, ""); rec.Code != http.StatusConflict || errorCode(t, rec) != "last_admin" {
		t.Fatalf("deactivate last remaining admin status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var systemID string
	if err := env.pool.QueryRow(context.Background(), "SELECT id::text FROM users WHERE username = 'system'").Scan(&systemID); err != nil {
		t.Fatalf("find system user: %v", err)
	}
	if rec := env.request(t, http.MethodPut, "/api/v1/users/"+systemID+"/roles", map[string]any{
		"roles": []string{"admin"},
	}, secondLogin.AccessToken, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("system roles status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := env.request(t, http.MethodPatch, "/api/v1/users/"+systemID+"/status", map[string]string{
		"status": "inactive",
	}, secondLogin.AccessToken, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("system status status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := env.request(t, http.MethodPost, "/api/v1/users/"+systemID+"/credential-reset", nil, secondLogin.AccessToken, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("system reset status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshRotationReuseAndLogout(t *testing.T) {
	env := newTestEnv(t)
	userID := env.insertUser(t, "rotation", "", "active", "rotation-password", "admin")

	login := env.login(t, "rotation", "rotation-password")
	if login.Cookie == nil {
		t.Fatal("login did not return refresh cookie")
	}

	rec := env.request(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", login.Cookie, testFrontendURL)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var rotated authResponse
	decode(t, rec, &rotated)
	if rotated.AccessToken == "" {
		t.Fatal("refresh did not return an access token")
	}
	newCookie := responseCookie(t, rec, "ifter_refresh")
	if newCookie == nil || newCookie.Value == login.Cookie.Value {
		t.Fatal("refresh token was not rotated")
	}

	// The old token is inside the concurrent-refresh grace window: reject it
	// without revoking the family.
	rec = env.request(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", login.Cookie, testFrontendURL)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("within-grace reuse status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertActiveRefreshTokens(t, env.pool, userID, 1)

	// Backdate the revoked token so the next reuse is outside the grace window.
	oldHash := hashForTest(login.Cookie.Value)
	if _, err := env.pool.Exec(context.Background(),
		"UPDATE refresh_tokens SET revoked_at = now() - interval '1 minute' WHERE token_hash = $1", oldHash,
	); err != nil {
		t.Fatalf("backdate rotated token: %v", err)
	}

	rec = env.request(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", login.Cookie, testFrontendURL)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reuse status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertActiveRefreshTokens(t, env.pool, userID, 0)
	assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'auth.refresh_reuse'", 1)

	rec = env.request(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", newCookie, testFrontendURL)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("rotation family refresh status = %d, body = %s", rec.Code, rec.Body.String())
	}

	login = env.login(t, "rotation", "rotation-password")
	rec = env.request(t, http.MethodPost, "/api/v1/auth/logout", nil, "", login.Cookie, testFrontendURL)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body = %s", rec.Code, rec.Body.String())
	}
	cleared := responseCookie(t, rec, "ifter_refresh")
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("logout did not clear refresh cookie: %+v", cleared)
	}
	rec = env.request(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", login.Cookie, testFrontendURL)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertCount(t, env.pool, "SELECT count(*) FROM audit_logs WHERE action = 'auth.logout'", 1)
}

func TestLoginRateLimit(t *testing.T) {
	env := newTestEnv(t)
	env.insertUser(t, "limited", "", "active", "limited-password", "teacher")
	limitedRouter := httptransport.New(discardLogger(), nil, env.service, nil, nil, nil, nil, nil, config.AuthConfig{
		AllowedOrigins:  []string{testFrontendURL},
		LoginRateLimit:  1,
		LoginRateWindow: time.Minute,
	}, nil)
	limited := testEnv{pool: env.pool, service: env.service, router: limitedRouter}

	rec := limited.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": "limited", "password": "limited-password",
	}, "", nil, testFrontendURL)
	if rec.Code != http.StatusOK {
		t.Fatalf("first login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = limited.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": "limited", "password": "limited-password",
	}, "", nil, testFrontendURL)
	if rec.Code != http.StatusTooManyRequests || errorCode(t, rec) != "rate_limited" {
		t.Fatalf("limited login status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAuthRequestValidationAndOrigin(t *testing.T) {
	env := newTestEnv(t)
	env.insertUser(t, "origin", "", "active", "origin-password", "teacher")

	rec := env.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": "origin", "password": "origin-password",
	}, "", nil, "http://evil.test")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = env.request(t, http.MethodOptions, "/api/v1/auth/login", nil, "", nil, testFrontendURL)
	if rec.Code != http.StatusNoContent ||
		rec.Header().Get("Access-Control-Allow-Origin") != testFrontendURL ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("CORS preflight = %d, headers = %#v", rec.Code, rec.Header())
	}

	rec = env.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": "origin", "password": "origin-password",
	}, "", nil, testFrontendURL)
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != testFrontendURL {
		t.Fatalf("allowed origin login = %d, headers = %#v", rec.Code, rec.Header())
	}

	rec = env.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{"login": "origin"}, "", nil, testFrontendURL)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing password status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = env.request(t, http.MethodPost, "/api/v1/auth/refresh", nil, "", nil, testFrontendURL)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing refresh cookie status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

type authResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresIn   int64        `json:"expires_in"`
	User        authpkg.User `json:"user"`
}

type loginResult struct {
	authResponse
	Cookie *http.Cookie
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	pool := newTestPool(t)
	applyUpMigrations(t, pool)

	tokens := authpkg.NewTokenManager(testSecret, "test-issuer", 15*time.Minute)
	service := authpkg.NewService(pool, tokens, authpkg.ServiceConfig{
		RefreshTokenTTL: 24 * time.Hour,
		InviteTokenTTL:  72 * time.Hour,
		ResetTokenTTL:   time.Hour,
		BcryptCost:      bcrypt.MinCost,
	}, discardLogger())
	router := httptransport.New(discardLogger(), nil, service, nil, nil, nil, nil, nil, config.AuthConfig{
		CookieSecure:    false,
		AllowedOrigins:  []string{testFrontendURL},
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return testEnv{pool: pool, service: service, router: router}
}

func (e testEnv) login(t *testing.T, login, password string) loginResult {
	t.Helper()
	rec := e.request(t, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"login": login, "password": password,
	}, "", nil, testFrontendURL)
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response authResponse
	decode(t, rec, &response)
	return loginResult{authResponse: response, Cookie: responseCookie(t, rec, "ifter_refresh")}
}

func (e testEnv) request(t *testing.T, method, path string, body any, accessToken string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func (e testEnv) insertUser(t *testing.T, username, email, status, password string, roles ...string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash test password: %v", err)
	}
	var emailValue any
	if email != "" {
		emailValue = email
	}
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash, full_name, status)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id::text`,
		username, emailValue, string(hash), "Test "+username, status,
	).Scan(&id); err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
	for _, role := range roles {
		if _, err := e.pool.Exec(context.Background(),
			"INSERT INTO user_roles (user_id, role_code) VALUES ($1, $2)", id, role,
		); err != nil {
			t.Fatalf("assign role %q: %v", role, err)
		}
	}
	return id
}

func newTestPool(t *testing.T) *pgxpool.Pool {
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

	name := fmt.Sprintf("ifter_auth_test_%d", time.Now().UnixNano())
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
	return pool
}

func applyUpMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
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
		if _, err := pool.Exec(context.Background(), string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", path, err)
		}
	}
}

func assertCount(t *testing.T, pool *pgxpool.Pool, query string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(), query).Scan(&got); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}

func assertActiveRefreshTokens(t *testing.T, pool *pgxpool.Pool, userID string, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL", userID,
	).Scan(&got); err != nil {
		t.Fatalf("count active refresh tokens: %v", err)
	}
	if got != want {
		t.Fatalf("active refresh tokens = %d, want %d", got, want)
	}
}

func responseCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	decode(t, rec, &body)
	return body.Code
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hashForTest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
