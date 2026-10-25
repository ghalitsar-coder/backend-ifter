package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationsAndCoreConstraints(t *testing.T) {
	adminURL := os.Getenv("TEST_DATABASE_URL")
	if adminURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	pool := newTestDatabase(t, adminURL)
	applyMigrations(t, pool, "up")

	t.Run("seed data", func(t *testing.T) {
		assertCount(t, pool, "SELECT count(*) FROM roles", 7)
		assertCount(t, pool, "SELECT count(*) FROM school_schedules", 5)

		var system bool
		if err := pool.QueryRow(context.Background(),
			"SELECT EXISTS (SELECT 1 FROM users WHERE username = 'system' AND status = 'system')",
		).Scan(&system); err != nil {
			t.Fatalf("check system user: %v", err)
		}
		if !system {
			t.Fatal("system user was not seeded")
		}
	})

	t.Run("case-insensitive username is unique", func(t *testing.T) {
		insertUser(t, pool, "Admin", "active", "hash")
		err := execErr(t, pool,
			"INSERT INTO users (username, password_hash, full_name, status) VALUES ($1, $2, $3, 'active')",
			"admin", "hash", "Admin Duplicate")
		if !IsUniqueViolation(err) {
			t.Fatalf("duplicate username error = %v, want unique violation", err)
		}
	})

	t.Run("active user requires password", func(t *testing.T) {
		err := execErr(t, pool,
			"INSERT INTO users (username, password_hash, full_name, status) VALUES ($1, NULL, $2, 'active')",
			"nopassword", "No Password")
		if !IsCheckViolation(err) {
			t.Fatalf("missing password error = %v, want check violation", err)
		}
	})

	t.Run("audit log is append-only", func(t *testing.T) {
		var id string
		if err := pool.QueryRow(context.Background(),
			`INSERT INTO audit_logs (action, entity_type)
			 VALUES ('test.action', 'test') RETURNING id::text`,
		).Scan(&id); err != nil {
			t.Fatalf("insert audit log: %v", err)
		}

		for _, query := range []string{
			"UPDATE audit_logs SET action = 'changed' WHERE id = $1",
			"DELETE FROM audit_logs WHERE id = $1",
		} {
			err := execErr(t, pool, query, id)
			if err == nil || !strings.Contains(err.Error(), "append-only") {
				t.Fatalf("audit mutation error = %v, want append-only error", err)
			}
		}

		if err := execErr(t, pool, "TRUNCATE audit_logs"); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("audit truncate error = %v, want append-only error", err)
		}
	})

	yearID := insertAcademicYear(t, pool, "2026/2027", "2026-07-01", "2027-06-30", true)

	t.Run("academic years cannot overlap", func(t *testing.T) {
		err := execErr(t, pool,
			"INSERT INTO academic_years (name, start_date, end_date) VALUES ($1, $2, $3)",
			"2026/2027-overlap", "2027-01-01", "2027-12-31")
		if !IsExclusionViolation(err) {
			t.Fatalf("overlapping academic year error = %v, want exclusion violation", err)
		}
	})

	t.Run("only one academic year is active", func(t *testing.T) {
		err := execErr(t, pool,
			"INSERT INTO academic_years (name, start_date, end_date, is_active) VALUES ($1, $2, $3, true)",
			"2027/2028", "2027-07-01", "2028-06-30")
		if !IsUniqueViolation(err) {
			t.Fatalf("second active academic year error = %v, want unique violation", err)
		}
	})

	t.Run("term must stay inside academic year", func(t *testing.T) {
		err := execErr(t, pool,
			`INSERT INTO terms (academic_year_id, term_number, name, start_date, end_date)
			 VALUES ($1, 1, 'Ganjil', $2, $3)`,
			yearID, "2026-06-01", "2026-12-31")
		if err == nil || !strings.Contains(err.Error(), "term dates must be within academic year") {
			t.Fatalf("term outside year error = %v, want trigger error", err)
		}
	})

	classID := insertClass(t, pool, yearID, "X-1")

	t.Run("enrollment cannot overlap", func(t *testing.T) {
		studentID := insertStudent(t, pool, "S001")
		insertEnrollment(t, pool, studentID, classID, "2026-08-01", "2026-12-31")

		err := execErr(t, pool,
			`INSERT INTO class_enrollments (student_id, class_id, start_date, end_date)
			 VALUES ($1, $2, $3, $4)`,
			studentID, classID, "2026-12-01", "2027-03-31")
		if !IsExclusionViolation(err) {
			t.Fatalf("overlapping enrollment error = %v, want exclusion violation", err)
		}

		insertEnrollment(t, pool, studentID, classID, "2027-01-01", "2027-06-30")
	})

	t.Run("enrollment must stay inside class academic year", func(t *testing.T) {
		studentID := insertStudent(t, pool, "S002")

		err := execErr(t, pool,
			`INSERT INTO class_enrollments (student_id, class_id, start_date, end_date)
			 VALUES ($1, $2, $3, $4)`,
			studentID, classID, "2027-07-01", "2027-08-01")
		if err == nil || !strings.Contains(err.Error(), "enrollment start date must be within class academic year") {
			t.Fatalf("enrollment outside year error = %v, want trigger error", err)
		}
	})

	t.Run("one homeroom teacher per class at a time", func(t *testing.T) {
		teacherA := insertTeacher(t, pool, "T001")
		teacherB := insertTeacher(t, pool, "T002")

		insertHomeroom(t, pool, teacherA, classID, "2026-08-01", "2026-12-31")

		err := execErr(t, pool,
			`INSERT INTO homeroom_assignments (teacher_id, class_id, start_date, end_date)
			 VALUES ($1, $2, $3, $4)`,
			teacherB, classID, "2026-11-01", "2027-01-31")
		if !IsExclusionViolation(err) {
			t.Fatalf("overlapping homeroom error = %v, want exclusion violation", err)
		}
	})

	t.Run("duty assignment cannot overlap on same weekday", func(t *testing.T) {
		teacher := insertTeacher(t, pool, "T003")

		exec(t, pool,
			`INSERT INTO duty_assignments (teacher_id, weekday, start_date, end_date)
			 VALUES ($1, 1, $2, $3)`,
			teacher, "2026-08-01", "2026-12-31")

		err := execErr(t, pool,
			`INSERT INTO duty_assignments (teacher_id, weekday, start_date, end_date)
			 VALUES ($1, 1, $2, $3)`,
			teacher, "2026-11-01", "2027-01-31")
		if !IsExclusionViolation(err) {
			t.Fatalf("overlapping duty error = %v, want exclusion violation", err)
		}
	})

	t.Run("transaction rolls back", func(t *testing.T) {
		err := WithTx(context.Background(), pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(context.Background(),
				`INSERT INTO students (student_number, full_name, admission_year)
				 VALUES ('TX001', 'Rollback Student', 2026)`,
			); err != nil {
				return err
			}
			return errors.New("force rollback")
		})
		if err == nil || err.Error() != "force rollback" {
			t.Fatalf("WithTx() error = %v, want force rollback", err)
		}
		assertCount(t, pool, "SELECT count(*) FROM students WHERE student_number = 'TX001'", 0)
	})

	t.Run("transaction commits", func(t *testing.T) {
		err := WithTx(context.Background(), pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(context.Background(),
				`INSERT INTO students (student_number, full_name, admission_year)
				 VALUES ('TX002', 'Commit Student', 2026)`,
			)
			return err
		})
		if err != nil {
			t.Fatalf("WithTx() error = %v", err)
		}
		assertCount(t, pool, "SELECT count(*) FROM students WHERE student_number = 'TX002'", 1)
	})

	applyMigrations(t, pool, "down")

	var usersTableExists bool
	if err := pool.QueryRow(context.Background(),
		"SELECT to_regclass('public.users') IS NOT NULL",
	).Scan(&usersTableExists); err != nil {
		t.Fatalf("check down migration: %v", err)
	}
	if usersTableExists {
		t.Fatal("users table still exists after down migration")
	}
}

func newTestDatabase(t *testing.T, adminURL string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	adminCfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, adminCfg)
	if err != nil {
		t.Fatalf("connect admin database: %v", err)
	}

	name := fmt.Sprintf("ifter_test_%d", time.Now().UnixNano())
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

	testCfg, err := pgxpool.ParseConfig(adminURL)
	if err != nil {
		t.Fatalf("parse test database config: %v", err)
	}
	testCfg.ConnConfig.Database = name
	testCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	pool, err = pgxpool.NewWithConfig(ctx, testCfg)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return pool
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool, direction string) {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "db", "migrations", "*."+direction+".sql"))
	if err != nil {
		t.Fatalf("find %s migrations: %v", direction, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no %s migrations found", direction)
	}
	sort.Strings(paths)
	if direction == "down" {
		for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
			paths[i], paths[j] = paths[j], paths[i]
		}
	}

	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if _, err := pool.Exec(context.Background(), string(body)); err != nil {
			t.Fatalf("apply %s: %v", path, err)
		}
	}
}

func insertAcademicYear(t *testing.T, pool *pgxpool.Pool, name, start, end string, active bool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO academic_years (name, start_date, end_date, is_active)
		 VALUES ($1, $2, $3, $4) RETURNING id::text`,
		name, start, end, active,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert academic year: %v", err)
	}
	return id
}

func insertClass(t *testing.T, pool *pgxpool.Pool, academicYearID, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO classes (academic_year_id, name, grade_level)
		 VALUES ($1, $2, 10) RETURNING id::text`,
		academicYearID, name,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert class: %v", err)
	}
	return id
}

func insertStudent(t *testing.T, pool *pgxpool.Pool, studentNumber string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO students (student_number, full_name, admission_year)
		 VALUES ($1, $2, 2026) RETURNING id::text`,
		studentNumber, "Student "+studentNumber,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert student: %v", err)
	}
	return id
}

func insertTeacher(t *testing.T, pool *pgxpool.Pool, employeeNumber string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(),
		`INSERT INTO teachers (employee_number, full_name)
		 VALUES ($1, $2) RETURNING id::text`,
		employeeNumber, "Teacher "+employeeNumber,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert teacher: %v", err)
	}
	return id
}

func insertEnrollment(t *testing.T, pool *pgxpool.Pool, studentID, classID, start, end string) {
	t.Helper()
	exec(t, pool,
		`INSERT INTO class_enrollments (student_id, class_id, start_date, end_date)
		 VALUES ($1, $2, $3, $4)`,
		studentID, classID, start, end)
}

func insertHomeroom(t *testing.T, pool *pgxpool.Pool, teacherID, classID, start, end string) {
	t.Helper()
	exec(t, pool,
		`INSERT INTO homeroom_assignments (teacher_id, class_id, start_date, end_date)
		 VALUES ($1, $2, $3, $4)`,
		teacherID, classID, start, end)
}

func insertUser(t *testing.T, pool *pgxpool.Pool, username, status, passwordHash string) {
	t.Helper()
	exec(t, pool,
		`INSERT INTO users (username, password_hash, full_name, status)
		 VALUES ($1, $2, $3, $4)`,
		username, passwordHash, "User "+username, status)
}

func exec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec query: %v", err)
	}
}

func execErr(t *testing.T, pool *pgxpool.Pool, query string, args ...any) error {
	t.Helper()
	_, err := pool.Exec(context.Background(), query, args...)
	return err
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
