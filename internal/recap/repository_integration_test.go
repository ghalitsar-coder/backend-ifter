package recap

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresStudentRecapIntegration(t *testing.T) {
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

	const month, year = 11, 2097
	date := time.Date(year, month, 10, 0, 0, 0, 0, time.UTC)
	var sessionID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO attendance_sessions (class_id, recorded_by, tanggal)
		VALUES (1, 1, $1)
		RETURNING id`, date).Scan(&sessionID)
	if err != nil {
		t.Fatalf("create isolated attendance session: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM student_attendance_recaps WHERE class_id = 1 AND bulan = $1 AND tahun = $2`, month, year)
		_, _ = pool.Exec(context.Background(), `DELETE FROM attendance_sessions WHERE id = $1`, sessionID)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO student_attendances (session_id, student_id, status) VALUES
			($1, 1, 'hadir'), ($1, 2, 'sakit'), ($1, 3, 'alpa')`, sessionID)
	if err != nil {
		t.Fatalf("create attendance rows: %v", err)
	}

	repo := NewPostgresRepository(pool)
	if err := repo.GenerateStudentRecaps(ctx, 1, month, year); err != nil {
		t.Fatalf("GenerateStudentRecaps() error = %v", err)
	}
	items, err := repo.ListStudentRecaps(ctx, 1, month, year)
	if err != nil {
		t.Fatalf("ListStudentRecaps() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d recap rows, want 3: %+v", len(items), items)
	}
	if items[0].TotalHadir+items[0].TotalSakit+items[0].TotalIzin+items[0].TotalAlpa != 1 {
		t.Errorf("first recap totals = %+v, want exactly one recorded status", items[0])
	}
	var recapID int64
	for _, item := range items {
		if item.StudentID == 1 {
			recapID = item.ID
			if item.TotalHadir != 1 || item.AttendanceRate != 100 || item.Status != "draft" {
				t.Errorf("student 1 recap = %+v, want hadir 1, rate 100, draft", item)
			}
		}
	}
	if recapID == 0 {
		t.Fatal("student 1 recap not found")
	}
	if otherSchool, err := repo.ListStudentRecaps(ctx, 999, month, year); err != nil || len(otherSchool) != 0 {
		t.Fatalf("ListStudentRecaps(other school) = %+v, %v; want empty, nil", otherSchool, err)
	}

	verified, err := repo.VerifyStudentRecap(ctx, recapID, 1, 2)
	if err != nil {
		t.Fatalf("VerifyStudentRecap() error = %v", err)
	}
	if verified.Status != "dikunci" || verified.VerifiedBy == nil || *verified.VerifiedBy != 2 {
		t.Fatalf("verification = %+v, want locked by TU user 2", verified)
	}

	if _, err := pool.Exec(ctx, `UPDATE student_attendances SET status = 'alpa' WHERE session_id = $1 AND student_id = 1`, sessionID); err != nil {
		t.Fatalf("change source attendance: %v", err)
	}
	if err := repo.GenerateStudentRecaps(ctx, 1, month, year); err != nil {
		t.Fatalf("regenerate recaps: %v", err)
	}
	items, err = repo.ListStudentRecaps(ctx, 1, month, year)
	if err != nil {
		t.Fatalf("list recaps after regeneration: %v", err)
	}
	for _, item := range items {
		if item.ID == recapID && (item.TotalHadir != 1 || item.TotalAlpa != 0 || item.Status != "dikunci") {
			t.Errorf("locked recap changed after generation: %+v", item)
		}
	}
}

func TestPostgresStaffRecapIntegration(t *testing.T) {
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

	const month, year = 11, 2098
	startsOn := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endsOn := startsOn.AddDate(0, 1, 0)
	var holidays []time.Time
	for day := startsOn; day.Before(endsOn); day = day.AddDate(0, 0, 1) {
		if day.Weekday() == time.Tuesday {
			holidays = append(holidays, day)
			break
		}
	}
	if len(holidays) == 0 {
		t.Fatal("test month has no Tuesday to use as a school holiday")
	}

	_, err = pool.Exec(ctx, `DELETE FROM staff_attendance_recaps WHERE bulan = $1 AND tahun = $2`, month, year)
	if err != nil {
		t.Fatalf("clean old recaps: %v", err)
	}
	_, err = pool.Exec(ctx, `DELETE FROM staff_attendances WHERE tanggal >= $1 AND tanggal < $2`, startsOn, endsOn)
	if err != nil {
		t.Fatalf("clean old staff attendance: %v", err)
	}
	_, err = pool.Exec(ctx, `DELETE FROM holidays WHERE academic_year_id = 1 AND tanggal >= $1 AND tanggal < $2`, startsOn, endsOn)
	if err != nil {
		t.Fatalf("clean old test holidays: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM staff_attendance_recaps WHERE bulan = $1 AND tahun = $2`, month, year)
		_, _ = pool.Exec(context.Background(), `DELETE FROM staff_attendances WHERE tanggal >= $1 AND tanggal < $2`, startsOn, endsOn)
		_, _ = pool.Exec(context.Background(), `DELETE FROM holidays WHERE academic_year_id = 1 AND tanggal >= $1 AND tanggal < $2`, startsOn, endsOn)
	})

	_, err = pool.Exec(ctx, `
		INSERT INTO holidays (academic_year_id, tanggal, keterangan)
		VALUES (1, $1, 'Integration test holiday')`, holidays[0])
	if err != nil {
		t.Fatalf("create test holiday: %v", err)
	}

	statuses := []string{"hadir", "terlambat", "izin", "sakit", "alpa"}
	date := startsOn
	for _, status := range statuses {
		for date.Weekday() == time.Saturday || date.Weekday() == time.Sunday || date.Equal(holidays[0]) {
			date = date.AddDate(0, 0, 1)
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO staff_attendances (staff_id, tanggal, status)
			VALUES (1, $1, $2)`, date, status)
		if err != nil {
			t.Fatalf("create %s attendance: %v", status, err)
		}
		date = date.AddDate(0, 0, 1)
	}

	expectedWorkdays := 0
	for day := startsOn; day.Before(endsOn); day = day.AddDate(0, 0, 1) {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday && !day.Equal(holidays[0]) {
			expectedWorkdays++
		}
	}

	repo := NewPostgresRepository(pool)
	if err := repo.GenerateStaffRecaps(ctx, 1, month, year); err != nil {
		t.Fatalf("GenerateStaffRecaps() error = %v", err)
	}
	items, err := repo.ListStaffRecaps(ctx, 1, month, year)
	if err != nil {
		t.Fatalf("ListStaffRecaps() error = %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d staff recaps, want four active demo staff: %+v", len(items), items)
	}
	var recapID int64
	for _, item := range items {
		switch item.StaffID {
		case 1:
			recapID = item.ID
			if item.TotalWorkdays != int32(expectedWorkdays) || item.TotalHadir != 1 || item.TotalTerlambat != 1 ||
				item.TotalIzin != 1 || item.TotalSakit != 1 || item.TotalAlpa != 1 || item.AttendanceRate != 10.53 {
				t.Errorf("staff 1 recap = %+v, expected workdays %d and all five statuses", item, expectedWorkdays)
			}
		case 2:
			if item.TotalHadir != 0 || item.TotalWorkdays != int32(expectedWorkdays) {
				t.Errorf("staff 2 recap = %+v, want zero attendance and expected workdays", item)
			}
		}
	}
	if recapID == 0 {
		t.Fatal("staff 1 recap not found")
	}
	if otherSchool, err := repo.ListStaffRecaps(ctx, 999, month, year); err != nil || len(otherSchool) != 0 {
		t.Fatalf("ListStaffRecaps(other school) = %+v, %v; want empty, nil", otherSchool, err)
	}

	verified, err := repo.VerifyStaffRecap(ctx, recapID, 1, 2)
	if err != nil {
		t.Fatalf("VerifyStaffRecap() error = %v", err)
	}
	if verified.Status != "dikunci" || verified.VerifiedBy == nil || *verified.VerifiedBy != 2 {
		t.Fatalf("verification = %+v, want locked by TU user 2", verified)
	}
	_, err = pool.Exec(ctx, `UPDATE staff_attendances SET status = 'alpa' WHERE staff_id = 1 AND tanggal = $1`, startsOn)
	if err != nil {
		t.Fatalf("change source attendance: %v", err)
	}
	if err := repo.GenerateStaffRecaps(ctx, 1, month, year); err != nil {
		t.Fatalf("regenerate recaps: %v", err)
	}
	items, err = repo.ListStaffRecaps(ctx, 1, month, year)
	if err != nil {
		t.Fatalf("list recaps after regeneration: %v", err)
	}
	for _, item := range items {
		if item.ID == recapID && (item.TotalHadir != 1 || item.Status != "dikunci") {
			t.Errorf("locked staff recap changed after generation: %+v", item)
		}
	}
}
