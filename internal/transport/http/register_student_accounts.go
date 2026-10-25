package httptransport

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	studentaccountspkg "github.com/ghalitsar-coder/backend-ifter/internal/studentaccounts"
	"github.com/ghalitsar-coder/backend-ifter/internal/transport/http/middleware"
)

// RegisterStudentAccounts wires two related surfaces for the student role:
//  1. admin provisioning of student login accounts (a new admin write surface),
//  2. the student's own /my/* portal (own attendance and own discipline).
//
// See docs/API_ADDENDUM.md §6.7 (external shape is authoritative) and the
// student-accounts package spec (migration 000015).
//
// Owner: student-accounts backend agent.
//
//	Provisioning surface (admin only, per ADDENDUM §1/§6.7) —
//	  POST   /api/v1/students/:id/account   { username?, email? }
//	    -> 201 { "user": {...}, "credential": { "token", "purpose", "expires_at" } }
//	       audited (student_account.create); the account is created and invited
//	       through the existing auth machinery (POST /users + /auth/activate),
//	       so no password is ever issued here. 409 when the student is already
//	       linked.
//	  GET    /api/v1/students/:id/account    -> 200 { "account": {...} }
//	  DELETE /api/v1/students/:id/account    -> 204 (login deactivated + sessions
//	       and unused invites revoked + link cleared; student history preserved),
//	       audited (student_account.delete)
//
//	groups.My (student only) — always scoped to the signed-in student's own row;
//	the use case resolves the student from the auth subject (students.user_id),
//	never from a client id, and answers 403 while no active student is linked:
//	  GET /api/v1/my/profile                    -> 200 { "profile": {...} }
//	  GET /api/v1/my/attendance?from=&to=&limit=&offset=
//	  GET /api/v1/my/attendance-history?...     (package-spec alias of the same view)
//	  GET /api/v1/my/attendance-summary?from=&to=
//	  GET /api/v1/my/discipline?from=&to=&limit=&offset=
//	  GET /api/v1/my/discipline-summary?from=&to=
//
// (The student's own dispensations + create-request endpoints live in
// register_dispensations.go, keeping dispensation domain logic in one place.
// GET /students/:id has no has_user_account field and GET /auth/me carries no
// student id: both responses belong to the masterdata/auth packages, which this
// agent does not own — GET /students/:id/account and GET /my/profile answer
// those needs instead.)
//
// Wiring note: this package does not own router.go; the service arrives
// through FeatureDeps. router.New() builds it on the shared API pool with the
// composition root's single auth.Service wrapped by the adapter below — one
// JWT secret, one pool, closed by the app on shutdown. Without a wired service
// (fake readiness, register tests) the surface stays disabled.

// authAccountProvisioner adapts the auth service (POST /users + invite
// credential machinery) to the studentaccounts.Provisioner contract, folding
// auth's sentinel errors into the student-accounts vocabulary so the handler
// keeps one error mapping and the domain package stays free of auth imports.
type authAccountProvisioner struct {
	service *auth.Service
}

func (p authAccountProvisioner) CreateAccount(ctx context.Context, actorID string, input studentaccountspkg.NewUserInput, requestID string) (studentaccountspkg.ProvisionedAccount, error) {
	account, credential, err := p.service.CreateUser(ctx, actorID, auth.CreateUserInput{
		Username: input.Username,
		Email:    input.Email,
		FullName: input.FullName,
		Roles:    input.Roles,
	}, requestID)
	if err != nil {
		return studentaccountspkg.ProvisionedAccount{}, mapAuthAccountError(err)
	}
	return studentaccountspkg.ProvisionedAccount{
		User: studentaccountspkg.UserAccount{
			ID:          account.ID,
			Username:    account.Username,
			Email:       account.Email,
			FullName:    account.FullName,
			Status:      account.Status,
			CreatedAt:   account.CreatedAt,
			LastLoginAt: account.LastLoginAt,
			Roles:       account.Roles,
		},
		Credential: studentaccountspkg.CredentialIssue{
			Token:     credential.Token,
			Purpose:   credential.Purpose,
			ExpiresAt: credential.ExpiresAt,
		},
	}, nil
}

func mapAuthAccountError(err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		return studentaccountspkg.ErrInvalidInput
	case errors.Is(err, auth.ErrUnauthorized):
		return studentaccountspkg.ErrUnauthorized
	case errors.Is(err, auth.ErrForbidden):
		return studentaccountspkg.ErrForbidden
	case errors.Is(err, auth.ErrNotFound):
		return studentaccountspkg.ErrNotFound
	case errors.Is(err, auth.ErrConflict):
		return studentaccountspkg.ErrConflict
	default:
		return err
	}
}

func RegisterStudentAccounts(v1 *gin.RouterGroup, groups RouteGroups, deps FeatureDeps) {
	_ = v1
	if groups.Admin == nil || groups.My == nil {
		return
	}
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	service := deps.StudentAccounts
	if service == nil {
		return
	}
	handler := newStudentAccountHandler(service, log)

	// Account provisioning is a sensitive write and is admin-only: the
	// docs/API_ADDENDUM.md §1 role table and §6.7 both put student-account
	// provisioning on groups.Admin (which already runs RequireRoles("admin")).
	// staff_tu is deliberately excluded; the domain guard
	// (studentaccounts.requireProvisionActor) enforces the same rule as defense
	// in depth.
	groups.Admin.POST("/students/:id/account", middleware.BodyLimit(64<<10), handler.Provision)
	groups.Admin.GET("/students/:id/account", handler.GetAccount)
	groups.Admin.DELETE("/students/:id/account", handler.Deactivate)

	groups.My.GET("/my/profile", handler.MyProfile)
	groups.My.GET("/my/attendance", handler.MyAttendance)
	groups.My.GET("/my/attendance-history", handler.MyAttendance)
	groups.My.GET("/my/attendance-summary", handler.MyAttendanceSummary)
	groups.My.GET("/my/discipline", handler.MyDiscipline)
	groups.My.GET("/my/discipline-summary", handler.MyDisciplineSummary)
}
