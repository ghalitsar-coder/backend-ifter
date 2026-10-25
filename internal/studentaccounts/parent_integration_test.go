package studentaccounts_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	studentaccountspkg "github.com/ghalitsar-coder/backend-ifter/internal/studentaccounts"
)

// TestParentPortalAPI covers migration 000019: the admin keeps the link set on
// PUT /users/:id/children, and a parent-role login reads its linked children
// through /parent/children* without ever confirming an unlinked student exists.
func TestParentPortalAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)

	parentID := env.insertUser(t, "sacc-parent", "sacc-parent@example.com", "sacc-parent-pw", "parent")
	parentToken := env.login(t, "sacc-parent", "sacc-parent-pw")
	env.insertUser(t, "sacc-parent2", "sacc-parent2@example.com", "sacc-parent2-pw", "parent")
	if _, err := env.pool.Exec(context.Background(),
		`INSERT INTO parent_students (user_id, student_id, relationship)
		 VALUES ($1, $2, 'mother')`, parentID, data.linkable,
	); err != nil {
		t.Fatalf("seed parent link: %v", err)
	}
	var staffID string
	if err := env.pool.QueryRow(context.Background(),
		"SELECT id::text FROM users WHERE username = 'sacc-staff'").Scan(&staffID); err != nil {
		t.Fatalf("find staff user: %v", err)
	}
	env.insert(t,
		`INSERT INTO dispensations (student_id, type, reason, start_date, status, submitted_by)
		 VALUES ($1, 'sick', 'Demam', '2026-09-09', 'approved', $2) RETURNING id::text`,
		data.linkable, data.adminID)

	adminPath := func(userID string) string { return "/api/v1/users/" + userID + "/children" }
	childPath := func(childID, suffix string) string {
		return "/api/v1/parent/children/" + childID + "/" + suffix
	}
	rangeQuery := "?from=2026-09-01&to=2026-09-30"

	type childList struct {
		Data []studentaccountspkg.ParentChild `json:"data"`
	}

	t.Run("link management is admin only", func(t *testing.T) {
		if rec := env.request(http.MethodGet, adminPath(parentID), nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous get status = %d", rec.Code)
		}
		for name, token := range map[string]string{
			"staff_tu": data.staffTUToken,
			"teacher":  data.teacherToken,
			"student":  data.orphanToken,
			"parent":   parentToken,
		} {
			payload := map[string]any{"children": []any{}}
			if rec := env.request(http.MethodGet, adminPath(parentID), nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s get status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
			if rec := env.request(http.MethodPut, adminPath(parentID), payload, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s put status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("admin reads the seeded link", func(t *testing.T) {
		rec := env.request(http.MethodGet, adminPath(parentID), nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("get status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("cache control = %q, want no-store", got)
		}
		var body childList
		decode(t, rec, &body)
		if len(body.Data) != 1 {
			t.Fatalf("children = %+v", body.Data)
		}
		child := body.Data[0]
		if child.StudentID != data.linkable || child.StudentNumber != "SAC-001" ||
			child.FullName != "Ani Berlink" || child.Status != "active" ||
			child.ClassName == nil || *child.ClassName != "12A" ||
			child.Relationship == nil || *child.Relationship != "mother" ||
			child.LinkedAt.IsZero() {
			t.Fatalf("child = %+v", child)
		}
	})

	t.Run("admin link validation", func(t *testing.T) {
		cases := []struct {
			name   string
			userID string
			body   any
			want   int
		}{
			{"malformed user id", "not-a-uuid", map[string]any{"children": []any{}}, http.StatusBadRequest},
			{"unknown user", "00000000-0000-4000-8000-000000000000", map[string]any{"children": []any{}}, http.StatusNotFound},
			{"target without parent role", staffID, map[string]any{"children": []any{}}, http.StatusConflict},
			{"missing children", parentID, map[string]any{}, http.StatusBadRequest},
			{"malformed student id", parentID, map[string]any{"children": []any{
				map[string]any{"student_id": "nope"},
			}}, http.StatusBadRequest},
			{"unknown student", parentID, map[string]any{"children": []any{
				map[string]any{"student_id": "00000000-0000-4000-8000-000000000000"},
			}}, http.StatusNotFound},
			{"duplicate student", parentID, map[string]any{"children": []any{
				map[string]any{"student_id": data.second},
				map[string]any{"student_id": data.second},
			}}, http.StatusBadRequest},
			{"invalid relationship", parentID, map[string]any{"children": []any{
				map[string]any{"student_id": data.second, "relationship": "uncle"},
			}}, http.StatusBadRequest},
			{"too many children", parentID, map[string]any{"children": tooManyChildren()}, http.StatusBadRequest},
		}
		for _, testCase := range cases {
			rec := env.request(http.MethodPut, adminPath(testCase.userID), testCase.body, data.adminToken)
			if rec.Code != testCase.want {
				t.Fatalf("%s status = %d, want %d, body = %s", testCase.name, rec.Code, testCase.want, rec.Body.String())
			}
		}
		if rec := env.requestRawString(http.MethodPut, adminPath(parentID), `{"children":`, data.adminToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed json status = %d", rec.Code)
		}
		// Every rejected payload left the seeded link untouched.
		var count int
		if err := env.pool.QueryRow(context.Background(),
			"SELECT count(*) FROM parent_students WHERE user_id = $1", parentID).Scan(&count); err != nil {
			t.Fatalf("count parent links: %v", err)
		}
		if count != 1 {
			t.Fatalf("rejected payloads changed the link set (%d rows)", count)
		}
	})

	t.Run("admin replaces the whole link set", func(t *testing.T) {
		rec := env.request(http.MethodPut, adminPath(parentID), map[string]any{"children": []any{}}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("clear status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var cleared childList
		decode(t, rec, &cleared)
		if len(cleared.Data) != 0 {
			t.Fatalf("cleared children = %+v", cleared.Data)
		}

		// Omitting relationship stores NULL (no tier is forced).
		rec = env.request(http.MethodPut, adminPath(parentID), map[string]any{"children": []any{
			map[string]any{"student_id": data.second},
		}}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("replace status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var replaced childList
		decode(t, rec, &replaced)
		if len(replaced.Data) != 1 || replaced.Data[0].StudentID != data.second ||
			replaced.Data[0].Relationship != nil {
			t.Fatalf("replaced children = %+v", replaced.Data)
		}

		// A graduated child can stay linked so the guardian keeps its history.
		rec = env.request(http.MethodPut, adminPath(parentID), map[string]any{"children": []any{
			map[string]any{"student_id": data.linkable, "relationship": "mother"},
			map[string]any{"student_id": data.graduate, "relationship": "guardian"},
		}}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("final replace status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var final childList
		decode(t, rec, &final)
		if len(final.Data) != 2 {
			t.Fatalf("final children = %+v", final.Data)
		}
		byID := make(map[string]studentaccountspkg.ParentChild, len(final.Data))
		for _, child := range final.Data {
			byID[child.StudentID] = child
		}
		graduate, ok := byID[data.graduate]
		if !ok || graduate.Status != "graduated" || graduate.ClassName != nil ||
			graduate.Relationship == nil || *graduate.Relationship != "guardian" {
			t.Fatalf("graduate = %+v", graduate)
		}
		if _, ok := byID[data.second]; ok {
			t.Fatal("replaced child is still linked")
		}
	})

	t.Run("every replacement is audited", func(t *testing.T) {
		var count int
		if err := env.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_logs
			 WHERE action = 'user.children.update' AND entity_type = 'user' AND entity_id = $1`,
			parentID).Scan(&count); err != nil {
			t.Fatalf("count audit rows: %v", err)
		}
		if count != 3 {
			t.Fatalf("audit rows = %d, want 3", count)
		}
		var before, after string
		if err := env.pool.QueryRow(context.Background(),
			`SELECT before_data::text, after_data::text FROM audit_logs
			 WHERE action = 'user.children.update' AND entity_id = $1
			 ORDER BY created_at DESC, id DESC LIMIT 1`, parentID).Scan(&before, &after); err != nil {
			t.Fatalf("read audit row: %v", err)
		}
		if !strings.Contains(before, data.second) {
			t.Fatalf("audit before = %s, want the replaced child", before)
		}
		for _, childID := range []string{data.linkable, data.graduate} {
			if !strings.Contains(after, childID) {
				t.Fatalf("audit after = %s, want %s", after, childID)
			}
		}
	})

	t.Run("parent reads its children", func(t *testing.T) {
		rec := env.request(http.MethodGet, "/api/v1/parent/children", nil, parentToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("children status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var body childList
		decode(t, rec, &body)
		if len(body.Data) != 2 {
			t.Fatalf("children = %+v", body.Data)
		}
		for _, child := range body.Data {
			if child.StudentID != data.linkable && child.StudentID != data.graduate {
				t.Fatalf("unexpected child %+v", child)
			}
		}

		// The other parent-role user has no links: an existing but unlinked
		// student is never confirmed (404, not 403 or an empty row).
		if rec := env.request(http.MethodGet, "/api/v1/parent/children", nil,
			env.login(t, "sacc-parent2", "sacc-parent2-pw")); rec.Code != http.StatusOK {
			t.Fatalf("unlinked parent status = %d", rec.Code)
		} else {
			var empty childList
			decode(t, rec, &empty)
			if len(empty.Data) != 0 {
				t.Fatalf("unlinked parent children = %+v", empty.Data)
			}
		}
	})

	t.Run("parent reads child attendance", func(t *testing.T) {
		rec := env.request(http.MethodGet, childPath(data.linkable, "attendance")+rangeQuery, nil, parentToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("attendance status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list studentaccountspkg.MyAttendanceList
		decode(t, rec, &list)
		if list.Total != 3 || len(list.Data) != 3 {
			t.Fatalf("attendance = %+v", list)
		}

		rec = env.request(http.MethodGet, childPath(data.linkable, "attendance-summary")+rangeQuery, nil, parentToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var summary studentaccountspkg.MyAttendanceSummary
		decode(t, rec, &summary)
		if len(summary.Data) != 1 || summary.Totals.TotalDays != 3 ||
			summary.Totals.Present != 1 || summary.Totals.Sick != 1 || summary.Totals.Absent != 1 ||
			summary.Data[0].StudentID != data.linkable {
			t.Fatalf("summary = %+v", summary)
		}
	})

	t.Run("parent reads child discipline", func(t *testing.T) {
		rec := env.request(http.MethodGet, childPath(data.linkable, "discipline")+rangeQuery, nil, parentToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("discipline status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list studentaccountspkg.MyDisciplineList
		decode(t, rec, &list)
		if list.Total != 3 || len(list.Data) != 3 {
			t.Fatalf("discipline = %+v", list)
		}

		rec = env.request(http.MethodGet, childPath(data.linkable, "discipline-summary")+rangeQuery, nil, parentToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("summary status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var summary studentaccountspkg.MyDisciplineSummary
		decode(t, rec, &summary)
		// Cancelled records keep their row but carry no points or sanction tier.
		if summary.TotalRecords != 2 || summary.TotalPoints != 12 {
			t.Fatalf("summary = %+v", summary)
		}
		if summary.CurrentLevel == nil || summary.CurrentLevel.Action != "Pemanggilan orang tua" ||
			summary.CurrentLevel.Level == nil || *summary.CurrentLevel.Level != "sedang" {
			t.Fatalf("current level = %+v", summary.CurrentLevel)
		}
	})

	t.Run("parent reads child dispensations", func(t *testing.T) {
		rec := env.request(http.MethodGet, childPath(data.linkable, "dispensations"), nil, parentToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("dispensations status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list studentaccountspkg.ParentDispensationList
		decode(t, rec, &list)
		if list.Total != 1 || len(list.Data) != 1 ||
			list.Data[0].Type != "sick" || list.Data[0].Status != "approved" ||
			list.Data[0].StartDate != "2026-09-09" || list.Data[0].EndDate != nil {
			t.Fatalf("dispensations = %+v", list)
		}
	})

	t.Run("unlinked child is not found", func(t *testing.T) {
		// second exists but belongs to no guardian's link set.
		for _, suffix := range []string{"attendance", "attendance-summary", "discipline", "discipline-summary"} {
			rec := env.request(http.MethodGet, childPath(data.second, suffix)+rangeQuery, nil, parentToken)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s unlinked status = %d, body = %s", suffix, rec.Code, rec.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			decode(t, rec, &body)
			if body.Code != "not_found" {
				t.Fatalf("%s error code = %q", suffix, body.Code)
			}
		}
		if rec := env.request(http.MethodGet, childPath(data.second, "dispensations"), nil, parentToken); rec.Code != http.StatusNotFound {
			t.Fatalf("dispensations unlinked status = %d", rec.Code)
		}
		if rec := env.request(http.MethodGet, childPath("not-a-uuid", "attendance")+rangeQuery, nil, parentToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed child status = %d", rec.Code)
		}
		if rec := env.request(http.MethodGet, childPath(data.linkable, "attendance"), nil, parentToken); rec.Code != http.StatusBadRequest {
			t.Fatalf("missing range status = %d", rec.Code)
		}
	})

	t.Run("parent routes reject other roles", func(t *testing.T) {
		if rec := env.request(http.MethodGet, "/api/v1/parent/children", nil, ""); rec.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous status = %d", rec.Code)
		}
		for name, token := range map[string]string{
			"admin":   data.adminToken,
			"staff":   data.staffTUToken,
			"teacher": data.teacherToken,
			"student": data.orphanToken,
		} {
			if rec := env.request(http.MethodGet, "/api/v1/parent/children", nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s parent children status = %d", name, rec.Code)
			}
			if rec := env.request(http.MethodGet, childPath(data.linkable, "attendance")+rangeQuery, nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s child attendance status = %d", name, rec.Code)
			}
		}
	})
}

// tooManyChildren builds one more link than the service accepts.
func tooManyChildren() []any {
	children := make([]any, 0, 21)
	for i := 0; i < 21; i++ {
		children = append(children, map[string]any{
			"student_id": fmt.Sprintf("00000000-0000-4000-8000-%012d", i),
		})
	}
	return children
}
