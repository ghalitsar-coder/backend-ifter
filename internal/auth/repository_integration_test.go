package auth

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
)

// Runs only when TEST_DATABASE_URL points at a migrated database with the demo
// seed loaded (db/seed/demo.sql). Locally:
//
//	TEST_DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' go test ./internal/auth/
func TestPostgresRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	t.Cleanup(pool.Close)

	var seeded bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE username = 'guru1')").Scan(&seeded); err != nil {
		t.Fatalf("check demo seed: %v", err)
	}
	if !seeded {
		t.Skip("demo seed is not loaded; run db/seed/demo.sql")
	}

	user, err := NewPostgresRepository(pool).FindByUsername(ctx, "guru1")
	if err != nil {
		t.Fatalf("FindByUsername() error = %v", err)
	}
	if user.ID != 1 || user.Username != "guru1" || user.Nama != "Ibu Sari" || !user.IsActive {
		t.Errorf("user = %+v, want the seeded guru with a staff name", user)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "guru" {
		t.Errorf("roles = %v, want [guru]", user.Roles)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("guru123")); err != nil {
		t.Errorf("demo seed password is not guru123: %v", err)
	}

	if _, err := NewPostgresRepository(pool).FindByUsername(ctx, "nobody"); !database.IsNotFound(err) {
		t.Errorf("unknown username error = %v, want a no-rows error", err)
	}
}

func TestPostgresRegisterSchoolAdminIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	t.Cleanup(pool.Close)

	var migrated bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.schools') IS NOT NULL").Scan(&migrated); err != nil {
		t.Fatalf("check schools table: %v", err)
	}
	if !migrated {
		t.Skip("schools migration is not applied")
	}

	repo := NewPostgresRepository(pool)
	suffix := time.Now().UnixNano() % 100000000
	npsn := fmt.Sprintf("%08d", suffix)
	email := fmt.Sprintf("admin%d@sekolah.test", suffix)

	in := RegisterInput{
		SchoolName: "SMP Uji Integrasi",
		NPSN:       npsn,
		Jenjang:    "SMP",
		AdminName:  "Admin Uji",
		Email:      email,
		Phone:      "081234567890",
		Password:   "rahasia123",
	}
	created, err := repo.RegisterSchoolAdmin(ctx, in, email, "hash")
	if err != nil {
		t.Fatalf("RegisterSchoolAdmin() error = %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM user_roles WHERE user_id = $1", created.UserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM staff WHERE user_id = $1", created.UserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", created.UserID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM schools WHERE id = $1", created.SchoolID)
	})

	if created.UserID == 0 || created.SchoolID == 0 || created.Username != email {
		t.Fatalf("created = %+v, want ids and the username", created)
	}

	var schoolName, jenjang string
	if err := pool.QueryRow(ctx, "SELECT nama, jenjang FROM schools WHERE id = $1", created.SchoolID).Scan(&schoolName, &jenjang); err != nil {
		t.Fatalf("read school: %v", err)
	}
	if schoolName != in.SchoolName || jenjang != "SMP" {
		t.Errorf("school = %q/%q, want %q/SMP", schoolName, jenjang, in.SchoolName)
	}

	var staffName, jabatan string
	if err := pool.QueryRow(ctx, "SELECT nama, jabatan FROM staff WHERE user_id = $1", created.UserID).Scan(&staffName, &jabatan); err != nil {
		t.Fatalf("read staff: %v", err)
	}
	if staffName != in.AdminName || jabatan != "Administrator" {
		t.Errorf("staff = %q/%q, want the admin staff row", staffName, jabatan)
	}

	var role string
	if err := pool.QueryRow(ctx, `
		SELECT r.kode FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1
	`, created.UserID).Scan(&role); err != nil {
		t.Fatalf("read role: %v", err)
	}
	if role != "admin" {
		t.Errorf("role = %q, want admin", role)
	}

	_, err = repo.RegisterSchoolAdmin(ctx, in, "other@sekolah.test", "hash")
	if !database.IsUniqueViolation(err) || database.ConstraintName(err) != "uq_schools_npsn" {
		t.Errorf("duplicate npsn error = %v, want uq_schools_npsn", err)
	}

	other := in
	other.NPSN = fmt.Sprintf("%08d", (suffix+1)%100000000)
	_, err = repo.RegisterSchoolAdmin(ctx, other, email, "hash")
	if !database.IsUniqueViolation(err) || database.ConstraintName(err) != "uq_users_username" {
		t.Errorf("duplicate email error = %v, want uq_users_username", err)
	}

	// The duplicate email call reuses the first admin's username, so one user
	// must remain: the original. The failed attempt must add no second row.
	for _, check := range []struct {
		label string
		param string
		query string
		want  int64
	}{{
		label: "school",
		param: other.NPSN,
		query: "SELECT count(*) FROM schools WHERE npsn = $1",
		want:  0,
	}, {
		label: "user",
		param: email,
		query: "SELECT count(*) FROM users WHERE username = $1",
		want:  1,
	}} {
		var rows int64
		if scanErr := pool.QueryRow(ctx, check.query, check.param).Scan(&rows); scanErr != nil {
			t.Fatalf("count rolled back %s: %v", check.label, scanErr)
		}
		if rows != check.want {
			t.Errorf("failed registration left %d %s rows, want %d; the CTE must be atomic", rows, check.label, check.want)
		}
	}
}
