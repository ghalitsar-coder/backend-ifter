package studentaccounts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestRequireProvisionActor(t *testing.T) {
	cases := []struct {
		name  string
		actor Actor
		want  error
	}{
		{"anonymous", Actor{}, ErrUnauthorized},
		{"blank id", Actor{ID: "   ", Roles: []string{RoleAdmin}}, ErrUnauthorized},
		{"admin", Actor{ID: actorID, Roles: []string{RoleAdmin}}, nil},
		{"staff_tu", Actor{ID: actorID, Roles: []string{RoleStaffTU}}, ErrForbidden},
		{"admin plus staff_tu", Actor{ID: actorID, Roles: []string{RoleStaffTU, RoleAdmin}}, nil},
		{"teacher", Actor{ID: actorID, Roles: []string{"teacher"}}, ErrForbidden},
		{"student", Actor{ID: actorID, Roles: []string{RoleStudent}}, ErrForbidden},
		{"principal without staff_tu", Actor{ID: actorID, Roles: []string{"principal"}}, ErrForbidden},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := requireProvisionActor(tt.actor); !errors.Is(err, tt.want) {
				t.Fatalf("requireProvisionActor(%+v) = %v, want %v", tt.actor, err, tt.want)
			}
		})
	}
}

func TestRequireStudentActor(t *testing.T) {
	cases := []struct {
		name  string
		actor Actor
		want  error
	}{
		{"anonymous", Actor{}, ErrUnauthorized},
		{"student", Actor{ID: actorID, Roles: []string{RoleStudent}}, nil},
		{"student among other roles", Actor{ID: actorID, Roles: []string{"teacher", RoleStudent}}, nil},
		{"admin", Actor{ID: actorID, Roles: []string{RoleAdmin}}, ErrForbidden},
		{"staff_tu", Actor{ID: actorID, Roles: []string{RoleStaffTU}}, ErrForbidden},
		{"no roles", Actor{ID: actorID}, ErrForbidden},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := requireStudentActor(tt.actor); !errors.Is(err, tt.want) {
				t.Fatalf("requireStudentActor(%+v) = %v, want %v", tt.actor, err, tt.want)
			}
		})
	}
}

func TestHasRole(t *testing.T) {
	roles := []string{RoleAdmin, RoleStudent}
	if !hasRole(roles, RoleStudent) {
		t.Fatal("hasRole missed an existing role")
	}
	if hasRole(roles, "principal") {
		t.Fatal("hasRole invented a role")
	}
	if hasRole(nil, RoleAdmin) {
		t.Fatal("hasRole on nil roles matched")
	}
	// Role codes are exact: staff_tu must not match on a prefix.
	if hasRole([]string{"staff"}, RoleStaffTU) {
		t.Fatal("hasRole matched a prefix instead of the code")
	}
}

func TestCleanUsername(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		err   error
	}{
		{"empty falls back to the student number", "", "", nil},
		{"blank falls back too", "   ", "", nil},
		{"surrounding space trimmed", "  s-1001  ", "s-1001", nil},
		{"embedded space rejected", "s 1001", "", ErrInvalidInput},
		{"tab rejected", "s\t1001", "", ErrInvalidInput},
		{"trailing newline trimmed", "s1001\n", "s1001", nil},
		{"nul byte rejected", "s100\x001", "", ErrInvalidInput},
		{"max length accepted", strings.Repeat("u", maxUsernameRunes), strings.Repeat("u", maxUsernameRunes), nil},
		{"over long rejected", strings.Repeat("u", maxUsernameRunes+1), "", ErrInvalidInput},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanUsername(tt.input)
			if !errors.Is(err, tt.err) {
				t.Fatalf("cleanUsername(%q) err = %v, want %v", tt.input, err, tt.err)
			}
			if got != tt.want {
				t.Fatalf("cleanUsername(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCleanEmail(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		err   error
	}{
		{"optional", "", "", nil},
		{"optional blank", "  ", "", nil},
		{"kept as supplied", "  ani@example.com ", "ani@example.com", nil},
		{"nul byte", "a\x00b@example.com", "", ErrInvalidInput},
		{"over long", strings.Repeat("a", maxEmailRunes+1), "", ErrInvalidInput},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cleanEmail(tt.input)
			if !errors.Is(err, tt.err) {
				t.Fatalf("cleanEmail(%q) err = %v, want %v", tt.input, err, tt.err)
			}
			if got != tt.want {
				t.Fatalf("cleanEmail(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseDateRange(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		bad  bool
	}{
		{"same day", "2026-09-01", "2026-09-01", false},
		{"normal window", "2026-07-01", "2026-09-30", false},
		{"max window", "2026-01-01", "2026-12-31", false},
		{"missing from", "", "2026-09-01", true},
		{"missing to", "2026-09-01", "", true},
		{"garbage from", "01-09-2026", "2026-09-30", true},
		{"garbage to", "2026-09-01", "30/09/2026", true},
		{"non-numeric", "2026-13-45", "2026-09-30", true},
		{"reversed bounds", "2026-09-30", "2026-09-01", true},
		{"window beyond cap", "2025-01-01", "2026-12-31", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			from, to, err := parseDateRange(tt.from, tt.to)
			if tt.bad {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("parseDateRange(%q,%q) err = %v, want ErrInvalidInput", tt.from, tt.to, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDateRange(%q,%q) err = %v", tt.from, tt.to, err)
			}
			if dateString(from) != strings.TrimSpace(tt.from) || dateString(to) != strings.TrimSpace(tt.to) {
				t.Fatalf("round trip = %q..%q, want %q..%q", dateString(from), dateString(to), tt.from, tt.to)
			}
		})
	}
}

func TestNormalizePage(t *testing.T) {
	cases := []struct{ limit, offset, wantLimit, wantOffset int32 }{
		{0, 0, 50, 0},
		{-5, -10, 50, 0},
		{10, 20, 10, 20},
		{100, 0, 100, 0},
		{101, 0, 100, 0},
		{5000, 3, 100, 3},
	}
	for _, tt := range cases {
		limit, offset := normalizePage(tt.limit, tt.offset)
		if limit != tt.wantLimit || offset != tt.wantOffset {
			t.Fatalf("normalizePage(%d,%d) = (%d,%d), want (%d,%d)",
				tt.limit, tt.offset, limit, offset, tt.wantLimit, tt.wantOffset)
		}
	}
}

func TestAttendancePercentage(t *testing.T) {
	cases := []struct {
		part, total int32
		want        float64
	}{
		{0, 0, 0},
		{5, 0, 0},
		{3, 4, 75},
		{1, 3, 33.33},
		{2, 3, 66.67},
		{4, 4, 100},
		{1, 8, 12.5},
	}
	for _, tt := range cases {
		if got := attendancePercentage(tt.part, tt.total); got != tt.want {
			t.Fatalf("attendancePercentage(%d,%d) = %v, want %v", tt.part, tt.total, got, tt.want)
		}
	}
}

func TestMapWriteError(t *testing.T) {
	errBoom := errors.New("boom")
	cases := []struct {
		name string
		in   error
		want error
	}{
		{"nil stays nil", nil, nil},
		{"sentinels pass through", ErrAlreadyLinked, ErrAlreadyLinked},
		{"unique", &pgconn.PgError{Code: "23505"}, ErrConflict},
		{"plain error passes through", errBoom, errBoom},
		{"exclusion", &pgconn.PgError{Code: "23P01"}, ErrConflict},
		{"foreign key", &pgconn.PgError{Code: "23503"}, ErrInvalidInput},
		{"check", &pgconn.PgError{Code: "23514"}, ErrInvalidInput},
		{"not null", &pgconn.PgError{Code: "23502"}, ErrInvalidInput},
		{"unknown driver error", pgx.ErrNoRows, pgx.ErrNoRows},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapWriteError(tt.in); !errors.Is(got, tt.want) {
				t.Fatalf("mapWriteError(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
	// The auth vocabulary folded by the register adapter must survive.
	for _, sentinel := range []error{ErrUnauthorized, ErrForbidden, ErrInvalidInput, ErrNotFound, ErrConflict, ErrAccountNotLinked} {
		if got := mapWriteError(sentinel); got != sentinel {
			t.Fatalf("mapWriteError(%v) = %v, want the sentinel intact", sentinel, got)
		}
	}
}

func TestConversionHelpers(t *testing.T) {
	if _, err := parseUUID("  "); err == nil {
		t.Fatal("parseUUID accepted a blank id")
	}
	id, err := parseUUID(actorID)
	if err != nil {
		t.Fatalf("parseUUID: %v", err)
	}
	if uuidString(id) != actorID {
		t.Fatalf("uuidString round trip = %q, want %q", uuidString(id), actorID)
	}
	if uuidString(pgtype.UUID{}) != "" {
		t.Fatal("invalid uuid rendered as non-empty")
	}
	if got := uuidPtr(pgtype.UUID{}); got != nil {
		t.Fatalf("uuidPtr(invalid) = %v, want nil", got)
	}
	if got := uuidPtr(id); got == nil || *got != actorID {
		t.Fatalf("uuidPtr(valid) = %v, want %q", got, actorID)
	}
	if got := textPtr(pgtype.Text{}); got != nil {
		t.Fatalf("textPtr(invalid) = %v, want nil", got)
	}
	named := "ani"
	if got := textPtr(pgtype.Text{String: named, Valid: true}); got == nil || *got != named {
		t.Fatalf("textPtr(valid) = %v, want %q", got, named)
	}
	if got := timestampPtr(pgtype.Timestamptz{}); got != nil {
		t.Fatalf("timestampPtr(invalid) = %v, want nil", got)
	}
	moment := time.Date(2026, 9, 1, 7, 15, 0, 0, time.UTC)
	if got := timestamp(pgtype.Timestamptz{Time: moment, Valid: true}); !got.Equal(moment) {
		t.Fatalf("timestamp = %v, want %v", got, moment)
	}
	if got := timestamp(pgtype.Timestamptz{}); !got.IsZero() {
		t.Fatalf("timestamp(invalid) = %v, want zero", got)
	}
	if got := timestampPtr(pgtype.Timestamptz{Time: moment, Valid: true}); got == nil || !got.Equal(moment) {
		t.Fatalf("timestampPtr(valid) = %v, want %v", got, moment)
	}
	if got := optionalText(""); got.Valid {
		t.Fatalf("optionalText(\"\") = %+v, want NULL", got)
	}
	if got := optionalText("req-1"); !got.Valid || got.String != "req-1" {
		t.Fatalf("optionalText = %+v", got)
	}
	if dateString(pgtype.Date{}) != "" {
		t.Fatal("dateString(invalid) rendered as non-empty")
	}
	if got := dateString(pgtype.Date{Time: moment, Valid: true}); got != "2026-09-01" {
		t.Fatalf("dateString = %q, want 2026-09-01", got)
	}
}

func TestNullableString(t *testing.T) {
	if got := nullableString(""); got != nil {
		t.Fatalf("nullableString(\"\") = %v, want nil", got)
	}
	if got := nullableString("   "); got != nil {
		t.Fatalf("nullableString(blank) = %v, want nil", got)
	}
	if got := nullableString(" 10A "); got == nil || *got != "10A" {
		t.Fatalf("nullableString = %v, want 10A", got)
	}
}

func TestAccountViewOf(t *testing.T) {
	view := accountViewOf("student-1", "SCT-001", "user-1", "sct-001", "pending")
	if view.StudentID != "student-1" || view.StudentNumber != "SCT-001" ||
		view.UserID != "user-1" || view.Username != "sct-001" || view.Status != "pending" {
		t.Fatalf("accountViewOf = %+v", view)
	}
	// A cleared link is rendered as empty strings, never as a stale user id.
	cleared := accountViewOf("student-1", "SCT-001", "", "sct-001", "inactive")
	if cleared.UserID != "" {
		t.Fatalf("cleared view = %+v", cleared)
	}
}

// The provisioning guards must all answer before any database is touched, so
// they are testable without a pool (the service is built with a nil one).
func TestProvisionGuardsDoNotTouchTheDatabase(t *testing.T) {
	ctx := context.Background()
	unlinkedStudentID := "11111111-1111-1111-1111-111111111111"
	fake := &fakeProvisioner{}

	cases := []struct {
		name    string
		service *Service
		actor   Actor
		input   ProvisionInput
		student string
		want    error
	}{
		{"teacher may not provision", serviceWithProvisioner(fake),
			Actor{ID: actorID, Roles: []string{"teacher"}}, ProvisionInput{}, unlinkedStudentID, ErrForbidden},
		{"student may not provision", serviceWithProvisioner(fake),
			Actor{ID: actorID, Roles: []string{RoleStudent}}, ProvisionInput{}, unlinkedStudentID, ErrForbidden},
		{"anonymous", serviceWithProvisioner(fake),
			Actor{}, ProvisionInput{}, unlinkedStudentID, ErrUnauthorized},
		{"actor id is not a uuid", serviceWithProvisioner(fake),
			Actor{ID: "root", Roles: []string{RoleAdmin}}, ProvisionInput{}, unlinkedStudentID, ErrUnauthorized},
		{"provisioner not wired", NewService(nil, nil, nil),
			Actor{ID: actorID, Roles: []string{RoleAdmin}}, ProvisionInput{}, unlinkedStudentID, ErrProvisionerUnavailable},
		{"student id is not a uuid", serviceWithProvisioner(fake),
			Actor{ID: actorID, Roles: []string{RoleAdmin}}, ProvisionInput{}, "not-a-uuid", ErrInvalidInput},
		{"username too long", serviceWithProvisioner(fake),
			Actor{ID: actorID, Roles: []string{RoleAdmin}},
			ProvisionInput{Username: strings.Repeat("u", maxUsernameRunes+1)}, unlinkedStudentID, ErrInvalidInput},
		{"username with a space", serviceWithProvisioner(fake),
			Actor{ID: actorID, Roles: []string{RoleAdmin}},
			ProvisionInput{Username: "ani widi"}, unlinkedStudentID, ErrInvalidInput},
		{"email too long", serviceWithProvisioner(fake),
			Actor{ID: actorID, Roles: []string{RoleAdmin}},
			ProvisionInput{Email: strings.Repeat("a", maxEmailRunes+1)}, unlinkedStudentID, ErrInvalidInput},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.service.Provision(ctx, tt.actor, tt.student, tt.input, "req-1")
			if !errors.Is(err, tt.want) {
				t.Fatalf("Provision err = %v, want %v", err, tt.want)
			}
			if result.User.ID != "" || result.Credential.Token != "" {
				t.Fatalf("guarded Provision returned a payload: %+v", result)
			}
		})
	}
	if fake.calls != 0 {
		t.Fatalf("provisioner called %d times by guarded requests, want 0", fake.calls)
	}
}

func TestReadAndDeactivateGuardsDoNotTouchTheDatabase(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil, &fakeProvisioner{}, nil)
	const studentID = "11111111-1111-1111-1111-111111111111"
	teacher := Actor{ID: actorID, Roles: []string{"teacher"}}
	admin := Actor{ID: actorID, Roles: []string{RoleAdmin}}

	if account, err := service.GetAccount(ctx, teacher, studentID); !errors.Is(err, ErrForbidden) || account.UserID != nil {
		t.Fatalf("GetAccount as teacher = (%+v, %v)", account, err)
	}
	if _, err := service.GetAccount(ctx, admin, "nope"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("GetAccount bad id err = %v", err)
	}
	if err := service.Deactivate(ctx, teacher, studentID, "req-1"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Deactivate as teacher err = %v", err)
	}
	if err := service.Deactivate(ctx, admin, "nope", "req-1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Deactivate bad id err = %v", err)
	}

	// The /my/* surface is student-only and resolves the student from the token.
	if _, err := service.MyProfile(ctx, admin); !errors.Is(err, ErrForbidden) {
		t.Fatalf("MyProfile as admin err = %v", err)
	}
	badSelf := Actor{ID: "not-a-uuid", Roles: []string{RoleStudent}}
	filter := DateRangeFilter{From: "2026-07-01", To: "2026-09-30"}
	if _, err := service.MyAttendance(ctx, badSelf, filter); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("MyAttendance with a broken subject err = %v", err)
	}
	if _, err := service.MyAttendanceSummary(ctx, teacher, filter); !errors.Is(err, ErrForbidden) {
		t.Fatalf("MyAttendanceSummary as teacher err = %v", err)
	}
	if _, err := service.MyDiscipline(ctx, teacher, filter); !errors.Is(err, ErrForbidden) {
		t.Fatalf("MyDiscipline as teacher err = %v", err)
	}
	if _, err := service.MyDisciplineSummary(ctx, Actor{}, filter); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("MyDisciplineSummary anonymous err = %v", err)
	}
}

const actorID = "0f0f0f0f-0f0f-0f0f-0f0f-0f0f0f0f0f0f"

type fakeProvisioner struct {
	calls int
	last  NewUserInput
	err   error
}

func (f *fakeProvisioner) CreateAccount(_ context.Context, actorID string, input NewUserInput, _ string) (ProvisionedAccount, error) {
	f.calls++
	f.last = input
	if f.err != nil {
		return ProvisionedAccount{}, f.err
	}
	return ProvisionedAccount{
		User:       UserAccount{ID: actorID, Username: input.Username, Roles: input.Roles, Status: "pending"},
		Credential: CredentialIssue{Token: "opaque", Purpose: "invite", ExpiresAt: time.Now().Add(time.Hour)},
	}, nil
}

func serviceWithProvisioner(provisioner Provisioner) *Service {
	return NewService(nil, provisioner, nil)
}
