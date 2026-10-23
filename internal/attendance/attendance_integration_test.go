package attendance_test

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
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"

	attendancepkg "github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	authpkg "github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/reporting"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

const (
	attendanceDay = "2026-09-02"
	holidayDay    = "2026-09-03"
	openDay       = "2026-09-09"
	autoCloseDay  = "2026-09-16"
	autoSkipDay   = "2026-09-23"
)

type apiEnv struct {
	pool    *pgxpool.Pool
	router  http.Handler
	service *attendancepkg.Service
}

type testData struct {
	yearID         string
	classID        string
	teacherID      string
	dutyID         string
	student1ID     string
	student2ID     string
	student3ID     string
	adminToken     string
	teacherToken   string
	dutyToken      string
	principalToken string
}

func TestAttendanceAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	t.Run("rbac and scope", func(t *testing.T) {
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance/roster?class_id="+data.classID+"&date="+attendanceDay, nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous roster status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance/roster?class_id="+data.classID+"&date="+attendanceDay, nil, data.principalToken); rec.Code != http.StatusOK {
			t.Fatalf("principal roster status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     attendanceDay,
			"records":  []map[string]string{{"student_id": data.student1ID, "status": "present"}},
		}, data.principalToken); rec.Code != http.StatusForbidden {
			t.Fatalf("principal save status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance/roster?class_id="+data.classID+"&date="+attendanceDay, nil, data.dutyToken); rec.Code != http.StatusOK {
			t.Fatalf("duty roster status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance/roster?class_id="+data.classID+"&date=2026-09-04", nil, data.dutyToken); rec.Code != http.StatusForbidden {
			t.Fatalf("duty non-duty roster status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("settings and holiday", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/attendance/settings", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("get settings status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var settings attendancepkg.SettingsDetail
		decode(t, rec, &settings)
		if settings.Timezone != "Asia/Jakarta" || len(settings.Schedules) != 5 {
			t.Fatalf("settings = %+v", settings)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance/settings", nil, data.teacherToken); rec.Code != http.StatusOK {
			t.Fatalf("teacher read settings status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/attendance/holidays", nil, data.teacherToken); rec.Code != http.StatusOK {
			t.Fatalf("teacher read holidays status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/attendance/settings", map[string]any{
			"school_name":                "Sekolah Test",
			"timezone":                   "Asia/Jakarta",
			"attendance_auto_close_time": "18:00",
			"schedules": []map[string]any{
				{"weekday": 1, "start_time": "07:00", "late_tolerance_minutes": 10},
				{"weekday": 2, "start_time": "07:00", "late_tolerance_minutes": 10},
				{"weekday": 3, "start_time": "07:00", "late_tolerance_minutes": 10},
				{"weekday": 4, "start_time": "07:00", "late_tolerance_minutes": 10},
				{"weekday": 5, "start_time": "07:00", "late_tolerance_minutes": 10},
			},
		}, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher settings status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/attendance/settings", map[string]any{
			"school_name":                "Sekolah Test",
			"timezone":                   "Asia/Jakarta",
			"attendance_auto_close_time": "18:00",
			"schedules": []map[string]any{
				{"weekday": 1, "start_time": "07:00", "late_tolerance_minutes": 10},
				{"weekday": 2, "start_time": "07:00", "late_tolerance_minutes": 10},
				{"weekday": 3, "start_time": "07:00", "late_tolerance_minutes": 10},
				{"weekday": 4, "start_time": "07:00", "late_tolerance_minutes": 10},
				{"weekday": 5, "start_time": "07:00", "late_tolerance_minutes": 10},
			},
		}, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("admin settings status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/attendance/holidays", map[string]string{
			"holiday_date": holidayDay,
			"name":         "Libur Uji",
		}, data.adminToken); rec.Code != http.StatusCreated {
			t.Fatalf("create holiday status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/attendance/roster?class_id="+data.classID+"&date="+holidayDay, nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("holiday roster status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var roster attendancepkg.Roster
		decode(t, rec, &roster)
		if roster.Effective {
			t.Fatalf("holiday roster should be ineffective: %+v", roster)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     holidayDay,
			"records":  []map[string]string{{"student_id": data.student1ID, "status": "present"}},
		}, data.teacherToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("holiday save status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodDelete, "/api/v1/attendance/holidays/"+holidayDay, nil, data.adminToken); rec.Code != http.StatusNoContent {
			t.Fatalf("delete holiday status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("duty assignment", func(t *testing.T) {
		payload := map[string]any{
			"teacher_id": data.teacherID,
			"weekday":    5,
			"start_date": "2026-10-01",
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/duty-assignments", nil, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher duty list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/duty-assignments", payload, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher duty create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec := env.request(t, http.MethodPost, "/api/v1/duty-assignments", payload, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("duty create status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var created attendancepkg.DutyAssignment
		decode(t, rec, &created)
		if created.Weekday != 5 || created.TeacherName != "Guru Wali" {
			t.Fatalf("duty assignment = %+v", created)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/duty-assignments", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("duty list status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var listed struct {
			Data []attendancepkg.DutyAssignment `json:"data"`
		}
		decode(t, rec, &listed)
		if len(listed.Data) != 2 {
			t.Fatalf("duty list = %+v", listed.Data)
		}
		rec = env.request(t, http.MethodPut, "/api/v1/duty-assignments/"+created.ID+"/end", map[string]string{
			"end_date": "2026-12-31",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("duty end status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("save roster and validation", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/attendance/roster?class_id="+data.classID+"&date="+attendanceDay, nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("roster status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var roster attendancepkg.Roster
		decode(t, rec, &roster)
		if !roster.Effective || roster.Summary.Total != 3 || roster.Summary.Unrecorded != 3 {
			t.Fatalf("initial roster = %+v", roster)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     attendanceDay,
			"records": []map[string]string{
				{"student_id": data.student1ID, "status": "present", "check_in_at": "07:05"},
				{"student_id": data.student2ID, "status": "late", "check_in_at": "07:20"},
			},
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("save status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &roster)
		if roster.Summary.Recorded != 2 || roster.Summary.Unrecorded != 1 || roster.Summary.Present != 1 || roster.Summary.Late != 1 {
			t.Fatalf("saved roster summary = %+v", roster.Summary)
		}

		rec = env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     attendanceDay,
			"records": []map[string]string{
				{"student_id": data.student1ID, "status": "sick", "notes": "demam"},
			},
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("partial save status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &roster)
		if roster.Summary.Sick != 1 || roster.Summary.Late != 1 || roster.Summary.Unrecorded != 1 {
			t.Fatalf("partial roster summary = %+v", roster.Summary)
		}

		for name, payload := range map[string]map[string]any{
			"unknown student": {"class_id": data.classID, "date": attendanceDay, "records": []map[string]string{{"student_id": "00000000-0000-0000-0000-000000000001", "status": "present"}}},
			"empty records":   {"class_id": data.classID, "date": attendanceDay, "records": []map[string]string{}},
			"invalid status":  {"class_id": data.classID, "date": attendanceDay, "records": []map[string]string{{"student_id": data.student1ID, "status": "unknown"}}},
			"absent checkin":  {"class_id": data.classID, "date": attendanceDay, "records": []map[string]string{{"student_id": data.student1ID, "status": "absent", "check_in_at": "07:00"}}},
		} {
			if rec := env.request(t, http.MethodPost, "/api/v1/attendance/bulk", payload, data.teacherToken); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("close, system absent, correction", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/attendance/close", map[string]string{
			"class_id": data.classID,
			"date":     attendanceDay,
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("close status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var closeResult attendancepkg.CloseResult
		decode(t, rec, &closeResult)
		if closeResult.Closure.CloseType != "manual" || closeResult.Summary.Absent != 1 || closeResult.Summary.Unrecorded != 0 {
			t.Fatalf("close result = %+v", closeResult)
		}
		var systemID string
		if err := env.pool.QueryRow(context.Background(), "SELECT id::text FROM users WHERE username='system'").Scan(&systemID); err != nil {
			t.Fatalf("get system user: %v", err)
		}
		var source, recordedBy string
		if err := env.pool.QueryRow(context.Background(),
			"SELECT source, recorded_by_user_id::text FROM attendance_records WHERE student_id=$1 AND attendance_date=$2",
			data.student3ID, attendanceDay,
		).Scan(&source, &recordedBy); err != nil {
			t.Fatalf("query system absent: %v", err)
		}
		if source != "system_close" || recordedBy != systemID {
			t.Fatalf("system absent = %q %q, want system_close %q", source, recordedBy, systemID)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/attendance/close", map[string]string{
			"class_id": data.classID,
			"date":     attendanceDay,
		}, data.teacherToken); rec.Code != http.StatusConflict {
			t.Fatalf("double close status = %d, body = %s", rec.Code, rec.Body.String())
		}

		roster := env.roster(t, data.teacherToken, data.classID, attendanceDay)
		var recordID string
		for _, student := range roster.Students {
			if student.StudentID == data.student1ID && student.Record != nil {
				recordID = student.Record.ID
			}
		}
		if recordID == "" {
			t.Fatal("student 1 record not found")
		}
		if _, err := env.pool.Exec(context.Background(),
			"UPDATE attendance_records SET status='present' WHERE id=$1", recordID,
		); err == nil {
			t.Fatal("direct update of closed attendance must fail")
		}
		rec = env.request(t, http.MethodPost, "/api/v1/attendance/records/"+recordID+"/corrections", map[string]any{
			"status":      "present",
			"check_in_at": "07:10",
			"notes":       "dikoreksi",
			"reason":      "salah input",
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("correction status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var detail attendancepkg.RecordDetail
		decode(t, rec, &detail)
		if detail.Status != "present" || len(detail.Corrections) != 1 {
			t.Fatalf("correction detail = %+v", detail)
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/attendance/records/"+recordID+"/corrections", map[string]any{
			"status": "late",
		}, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("correction without reason status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("open day correction rejected", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     openDay,
			"records":  []map[string]string{{"student_id": data.student1ID, "status": "present"}},
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("open save status = %d, body = %s", rec.Code, rec.Body.String())
		}
		roster := env.roster(t, data.teacherToken, data.classID, openDay)
		var recordID string
		for _, student := range roster.Students {
			if student.StudentID == data.student1ID && student.Record != nil {
				recordID = student.Record.ID
			}
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/attendance/records/"+recordID+"/corrections", map[string]any{
			"status": "late",
			"reason": "belum ditutup",
		}, data.teacherToken); rec.Code != http.StatusConflict {
			t.Fatalf("open correction status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("summary, history, export", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/attendance/summary?from="+attendanceDay+"&to="+attendanceDay, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var summary attendancepkg.SummaryResult
		decode(t, rec, &summary)
		if len(summary.Data) != 3 || summary.Totals.TotalDays != 3 || summary.Totals.Present != 1 || summary.Totals.Late != 1 || summary.Totals.Absent != 1 {
			t.Fatalf("summary = %+v", summary)
		}
		if summary.Totals.Percentage != 66.67 {
			t.Fatalf("summary percentage = %v", summary.Totals.Percentage)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/attendance/history?from="+attendanceDay+"&to="+attendanceDay, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("history status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var history attendancepkg.HistoryResult
		decode(t, rec, &history)
		if history.Total != 3 || len(history.Data) != 3 || !history.Data[0].Closed {
			t.Fatalf("history = %+v", history)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/exports/attendance?from="+attendanceDay+"&to="+attendanceDay, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("export status = %d, body = %s", rec.Code, rec.Body.String())
		}
		file, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("open export: %v", err)
		}
		defer func() { _ = file.Close() }()
		rows, err := file.GetRows("Rekap Absensi")
		if err != nil {
			t.Fatalf("read export rows: %v", err)
		}
		if len(rows) != 5 || rows[0][0] != "NIS" || rows[len(rows)-1][0] != "Total" {
			t.Fatalf("export rows = %#v", rows)
		}
	})

	t.Run("dashboard, reports, audit logs", func(t *testing.T) {
		rec := env.request(t, http.MethodGet, "/api/v1/dashboard?date="+attendanceDay, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin dashboard status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var dashboard reporting.Dashboard
		decode(t, rec, &dashboard)
		if dashboard.Role != "admin" || dashboard.Attendance.TotalStudents != 3 ||
			dashboard.Attendance.Present != 1 || dashboard.Attendance.Late != 1 || dashboard.Attendance.Absent != 1 {
			t.Fatalf("admin dashboard = %+v", dashboard)
		}
		if dashboard.ActiveStudents == nil || *dashboard.ActiveStudents != 3 ||
			dashboard.ActiveTeachers == nil || *dashboard.ActiveTeachers != 2 {
			t.Fatalf("dashboard active counts = %+v %+v", dashboard.ActiveStudents, dashboard.ActiveTeachers)
		}
		if len(dashboard.Trend) != 7 || len(dashboard.RecentActivity) == 0 {
			t.Fatalf("dashboard trend/recent = %d %d", len(dashboard.Trend), len(dashboard.RecentActivity))
		}

		rec = env.request(t, http.MethodGet, "/api/v1/dashboard?date="+attendanceDay, nil, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("teacher dashboard status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var teacherDashboard reporting.Dashboard
		decode(t, rec, &teacherDashboard)
		if teacherDashboard.Role != "teacher" || teacherDashboard.Attendance.TotalStudents != 3 || teacherDashboard.ActiveStudents != nil {
			t.Fatalf("teacher dashboard = %+v", teacherDashboard)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/dashboard?date="+attendanceDay, nil, data.dutyToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("duty dashboard status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var dutyDashboard reporting.Dashboard
		decode(t, rec, &dutyDashboard)
		if dutyDashboard.Role != "duty_teacher" || dutyDashboard.Attendance.TotalStudents != 3 {
			t.Fatalf("duty dashboard = %+v", dutyDashboard)
		}

		if rec := env.request(t, http.MethodGet, "/api/v1/audit-logs", nil, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher audit status = %d, body = %s", rec.Code, rec.Body.String())
		}
		rec = env.request(t, http.MethodGet, "/api/v1/audit-logs?entity_type=school_holiday&limit=10", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("admin audit status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var logs reporting.AuditLogResult
		decode(t, rec, &logs)
		if logs.Total == 0 || len(logs.Data) == 0 {
			t.Fatalf("audit logs = %+v", logs)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/audit-logs?from=2000-01-01&to=2099-12-31", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("audit date filter status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var filteredLogs reporting.AuditLogResult
		decode(t, rec, &filteredLogs)
		if filteredLogs.Total == 0 {
			t.Fatalf("audit date filter returned no rows")
		}
		rec = env.request(t, http.MethodGet, "/api/v1/audit-logs?from=2099-01-01", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("audit future filter status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var emptyLogs reporting.AuditLogResult
		decode(t, rec, &emptyLogs)
		if emptyLogs.Total != 0 {
			t.Fatalf("audit future filter total = %d", emptyLogs.Total)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/reports/attendance?from="+attendanceDay+"&to="+attendanceDay+"&group_by=day", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("attendance report status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var report attendancepkg.ReportResult
		decode(t, rec, &report)
		if len(report.Data) != 1 || report.Data[0].ClassName != "10A" ||
			report.Data[0].TotalDays != 3 || report.Data[0].Present != 1 || report.Data[0].Late != 1 ||
			report.Data[0].Absent != 1 || report.Totals.Percentage != 66.67 {
			t.Fatalf("attendance report = %+v", report)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/reports/attendance?from="+attendanceDay+"&to="+attendanceDay+"&group_by=year", nil, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid report group status = %d, body = %s", rec.Code, rec.Body.String())
		}

		var termID string
		if err := env.pool.QueryRow(context.Background(),
			`INSERT INTO terms (academic_year_id, term_number, name, start_date, end_date)
			 VALUES ($1, 1, 'Ganjil Test', '2026-09-01', '2026-09-30') RETURNING id::text`,
			data.yearID,
		).Scan(&termID); err != nil {
			t.Fatalf("insert report term: %v", err)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/reports/attendance?term_id="+termID+"&group_by=month", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("term report status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var termReport attendancepkg.ReportResult
		decode(t, rec, &termReport)
		if termReport.From != "2026-09-01" || termReport.To != "2026-09-30" || len(termReport.Data) == 0 {
			t.Fatalf("term report = %+v", termReport)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/exports/attendance-report?from="+attendanceDay+"&to="+attendanceDay+"&group_by=day", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("attendance report export status = %d, body = %s", rec.Code, rec.Body.String())
		}
		file, err := excelize.OpenReader(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("open report export: %v", err)
		}
		defer func() { _ = file.Close() }()
		rows, err := file.GetRows("Laporan Absensi")
		if err != nil {
			t.Fatalf("read report export rows: %v", err)
		}
		if len(rows) != 3 || rows[0][0] != "Periode Mulai" || rows[len(rows)-1][0] != "Total" {
			t.Fatalf("report export rows = %#v", rows)
		}
	})

	t.Run("auto close", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/attendance/bulk", map[string]any{
			"class_id": data.classID,
			"date":     autoCloseDay,
			"records":  []map[string]string{{"student_id": data.student1ID, "status": "present"}},
		}, data.teacherToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("auto save status = %d, body = %s", rec.Code, rec.Body.String())
		}
		loc, err := time.LoadLocation("Asia/Jakarta")
		if err != nil {
			t.Fatalf("load timezone: %v", err)
		}
		if err := env.service.AutoCloseDue(context.Background(), time.Date(2026, 9, 23, 17, 59, 0, 0, loc)); err != nil {
			t.Fatalf("early auto close: %v", err)
		}
		var count int
		if err := env.pool.QueryRow(context.Background(), "SELECT count(*) FROM attendance_day_closures WHERE attendance_date=$1", autoSkipDay).Scan(&count); err != nil {
			t.Fatalf("count early closure: %v", err)
		}
		if count != 0 {
			t.Fatalf("auto close ran before close time: %d", count)
		}
		if err := env.service.AutoCloseDue(context.Background(), time.Date(2026, 9, 16, 18, 0, 0, 0, loc)); err != nil {
			t.Fatalf("auto close: %v", err)
		}
		roster := env.roster(t, data.teacherToken, data.classID, autoCloseDay)
		if roster.Closure == nil || roster.Closure.CloseType != "auto" || roster.Summary.Absent != 2 || roster.Summary.Unrecorded != 0 {
			t.Fatalf("auto close roster = %+v", roster)
		}
	})

	t.Run("reporting performance smoke", func(t *testing.T) {
		ctx := context.Background()
		prefix := fmt.Sprintf("PERF%d", time.Now().UnixNano())
		if _, err := env.pool.Exec(ctx,
			`INSERT INTO students (student_number, full_name, admission_year)
			 SELECT $1 || '-' || g, 'Perf Student ' || g, 2026 FROM generate_series(1, 200) g`,
			prefix,
		); err != nil {
			t.Fatalf("seed performance students: %v", err)
		}
		if _, err := env.pool.Exec(ctx,
			`INSERT INTO class_enrollments (student_id, class_id, start_date, end_date)
			 SELECT s.id, $1, '2026-09-01', '2027-06-30'
			 FROM students s
			 WHERE s.student_number LIKE $2`,
			data.classID, prefix+"-%",
		); err != nil {
			t.Fatalf("seed performance enrollments: %v", err)
		}
		if _, err := env.pool.Exec(ctx,
			`INSERT INTO attendance_records
			   (student_id, enrollment_id, attendance_date, status, source, recorded_by_user_id, updated_by_user_id)
			 SELECT e.student_id, e.id, d::date, 'present', 'manual',
			        (SELECT id FROM users WHERE username = 'att-admin'),
			        (SELECT id FROM users WHERE username = 'att-admin')
			 FROM class_enrollments e
			 CROSS JOIN generate_series('2027-01-01'::date, '2027-01-20'::date, interval '1 day') d
			 WHERE e.class_id = $1 AND e.start_date = '2026-09-01' AND e.end_date = '2027-06-30'`,
			data.classID,
		); err != nil {
			t.Fatalf("seed performance attendance: %v", err)
		}

		start := time.Now()
		rec := env.request(t, http.MethodGet, "/api/v1/dashboard?date=2027-01-10", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("performance dashboard status = %d, body = %s", rec.Code, rec.Body.String())
		}
		dashboardDuration := time.Since(start)

		start = time.Now()
		rec = env.request(t, http.MethodGet, "/api/v1/reports/attendance?from=2027-01-01&to=2027-01-20&group_by=day", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("performance report status = %d, body = %s", rec.Code, rec.Body.String())
		}
		reportDuration := time.Since(start)

		t.Logf("dashboard=%s report=%s", dashboardDuration, reportDuration)
		if dashboardDuration > 5*time.Second || reportDuration > 5*time.Second {
			t.Fatalf("reporting performance too slow: dashboard=%s report=%s", dashboardDuration, reportDuration)
		}
	})
}

func (e apiEnv) seed(t *testing.T) testData {
	t.Helper()
	adminID := e.insertUser(t, "att-admin", "att-admin@example.com", "att-admin-password", "admin")
	teacherID := e.insertUser(t, "att-teacher", "att-teacher@example.com", "att-teacher-password", "teacher")
	dutyID := e.insertUser(t, "att-duty", "att-duty@example.com", "att-duty-password", "duty_teacher")
	principalID := e.insertUser(t, "att-principal", "att-principal@example.com", "att-principal-password", "principal")
	_ = adminID
	_ = principalID
	yearID := e.insertAcademicYear(t, "2026/2027", "2026-07-01", "2027-06-30")
	classID := e.insertClass(t, yearID, "10A", 10, "IPA")
	teacherProfileID := e.insertTeacher(t, teacherID, "Guru Wali")
	dutyProfileID := e.insertTeacher(t, dutyID, "Guru Piket")
	e.insertHomeroom(t, teacherProfileID, classID)
	e.insertDuty(t, dutyProfileID, 3)
	student1ID := e.insertStudent(t, "A001", "Ani")
	student2ID := e.insertStudent(t, "A002", "Budi")
	student3ID := e.insertStudent(t, "A003", "Citra")
	e.insertEnrollment(t, student1ID, classID)
	e.insertEnrollment(t, student2ID, classID)
	e.insertEnrollment(t, student3ID, classID)
	return testData{
		yearID:         yearID,
		classID:        classID,
		teacherID:      teacherProfileID,
		dutyID:         dutyProfileID,
		student1ID:     student1ID,
		student2ID:     student2ID,
		student3ID:     student3ID,
		adminToken:     e.login(t, "att-admin", "att-admin-password"),
		teacherToken:   e.login(t, "att-teacher", "att-teacher-password"),
		dutyToken:      e.login(t, "att-duty", "att-duty-password"),
		principalToken: e.login(t, "att-principal", "att-principal-password"),
	}
}

func (e apiEnv) roster(t *testing.T, token, classID, date string) attendancepkg.Roster {
	t.Helper()
	rec := e.request(t, http.MethodGet, "/api/v1/attendance/roster?class_id="+classID+"&date="+date, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("roster status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result attendancepkg.Roster
	decode(t, rec, &result)
	return result
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
	name := fmt.Sprintf("ifter_attendance_test_%d", time.Now().UnixNano())
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
	service := attendancepkg.NewService(pool, log)
	reportingService := reporting.NewService(pool, log)
	router := httptransport.New(log, nil, authService, nil, nil, service, nil, reportingService, config.AuthConfig{
		LoginRateLimit:  1000,
		LoginRateWindow: time.Minute,
	}, nil)
	return apiEnv{pool: pool, router: router, service: service}
}

func (e apiEnv) insertUser(t *testing.T, username, email, password string, roles ...string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO users (username, email, password_hash, full_name, status)
		 VALUES ($1, $2, $3, $4, 'active') RETURNING id::text`,
		username, email, string(hash), "Test "+username,
	).Scan(&id); err != nil {
		t.Fatalf("insert user %q: %v", username, err)
	}
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

func (e apiEnv) insertAcademicYear(t *testing.T, name, start, end string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO academic_years (name, start_date, end_date, is_active)
		 VALUES ($1,$2,$3,true) RETURNING id::text`, name, start, end,
	).Scan(&id); err != nil {
		t.Fatalf("insert academic year: %v", err)
	}
	return id
}

func (e apiEnv) insertClass(t *testing.T, yearID, name string, grade int, major string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO classes (academic_year_id, name, grade_level, major)
		 VALUES ($1,$2,$3,$4) RETURNING id::text`, yearID, name, grade, major,
	).Scan(&id); err != nil {
		t.Fatalf("insert class: %v", err)
	}
	return id
}

func (e apiEnv) insertTeacher(t *testing.T, userID, name string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO teachers (user_id, full_name) VALUES ($1,$2) RETURNING id::text`, userID, name,
	).Scan(&id); err != nil {
		t.Fatalf("insert teacher: %v", err)
	}
	return id
}

func (e apiEnv) insertHomeroom(t *testing.T, teacherID, classID string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO homeroom_assignments (teacher_id, class_id, start_date) VALUES ($1,$2,'2026-07-01')`,
		teacherID, classID,
	); err != nil {
		t.Fatalf("insert homeroom: %v", err)
	}
}

func (e apiEnv) insertDuty(t *testing.T, teacherID string, weekday int) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO duty_assignments (teacher_id, weekday, start_date) VALUES ($1,$2,'2026-07-01')`,
		teacherID, weekday,
	); err != nil {
		t.Fatalf("insert duty: %v", err)
	}
}

func (e apiEnv) insertStudent(t *testing.T, number, name string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(),
		`INSERT INTO students (student_number, full_name, admission_year, status)
		 VALUES ($1,$2,2026,'active') RETURNING id::text`, number, name,
	).Scan(&id); err != nil {
		t.Fatalf("insert student: %v", err)
	}
	return id
}

func (e apiEnv) insertEnrollment(t *testing.T, studentID, classID string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(),
		`INSERT INTO class_enrollments (student_id, class_id, start_date) VALUES ($1,$2,'2026-07-01')`,
		studentID, classID,
	); err != nil {
		t.Fatalf("insert enrollment: %v", err)
	}
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
