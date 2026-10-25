package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
)

// TestRequireRoles locks the authorization primitive every capability sub-group
// in router.go is built from: a signed-in user is allowed if they hold at least
// one of the required role codes, and gets 403 otherwise (401 when no user).
func TestRequireRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		required   []string
		userRoles  []string
		setUser    bool
		wantStatus int
	}{
		{name: "admin allowed on admin group", required: []string{"admin"}, userRoles: []string{"admin"}, setUser: true, wantStatus: http.StatusOK},
		{name: "staff_tu allowed on masterdata group", required: []string{"admin", "staff_tu"}, userRoles: []string{"staff_tu"}, setUser: true, wantStatus: http.StatusOK},
		{name: "teacher allowed on attendance ops", required: []string{"admin", "teacher", "duty_teacher"}, userRoles: []string{"teacher"}, setUser: true, wantStatus: http.StatusOK},
		{name: "principal denied attendance ops", required: []string{"admin", "teacher", "duty_teacher"}, userRoles: []string{"principal"}, setUser: true, wantStatus: http.StatusForbidden},
		{name: "principal allowed attendance read", required: []string{"admin", "teacher", "duty_teacher", "principal"}, userRoles: []string{"principal"}, setUser: true, wantStatus: http.StatusOK},
		{name: "student denied staff read", required: []string{"admin", "teacher", "duty_teacher", "principal"}, userRoles: []string{"student"}, setUser: true, wantStatus: http.StatusForbidden},
		{name: "staff_tu denied user management", required: []string{"admin"}, userRoles: []string{"staff_tu"}, setUser: true, wantStatus: http.StatusForbidden},
		{name: "multi-role user with one match allowed", required: []string{"principal"}, userRoles: []string{"teacher", "principal"}, setUser: true, wantStatus: http.StatusOK},
		{name: "missing user unauthorized", required: []string{"admin"}, userRoles: nil, setUser: false, wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)

			if tt.setUser {
				c.Set(userContextKey, auth.User{Roles: tt.userRoles})
			}

			handler := RequireRoles(tt.required...)
			handler(c)

			if c.IsAborted() && tt.wantStatus == http.StatusOK {
				t.Fatalf("unexpectedly aborted for an allowed role set")
			}
			if !c.IsAborted() && tt.wantStatus != http.StatusOK {
				t.Fatalf("expected request to be blocked with %d, but it was allowed", tt.wantStatus)
			}
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
