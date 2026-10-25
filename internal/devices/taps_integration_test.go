package devices_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type deviceCreateResponse struct {
	Device      map[string]any `json:"device"`
	IngestToken string         `json:"ingest_token"`
}

type tapIngestResponse struct {
	Accepted   int `json:"accepted"`
	Duplicates int `json:"duplicates"`
	Unknown    int `json:"unknown_cards"`
}

type tapListResponse struct {
	Data   []map[string]any `json:"data"`
	Total  int64            `json:"total"`
	Limit  int32            `json:"limit"`
	Offset int32            `json:"offset"`
}

type cardResponse struct {
	StudentCard struct {
		StudentID string  `json:"student_id"`
		CardUID   *string `json:"card_uid"`
	} `json:"student_card"`
}

func TestDeviceTapIngestAPI(t *testing.T) {
	env := newAPIEnv(t)
	data := env.seed(t)
	env.insertUser(t, "dev-duty", "dev-duty@example.com", "dev-duty-pw", "duty_teacher")
	dutyToken := env.login(t, "dev-duty", "dev-duty-pw")

	ctx := context.Background()
	insertStudent := func(t *testing.T, number, name string) string {
		t.Helper()
		var id string
		if err := env.pool.QueryRow(ctx,
			`INSERT INTO students (student_number, full_name, admission_year)
			 VALUES ($1, $2, 2026) RETURNING id::text`, number, name,
		).Scan(&id); err != nil {
			t.Fatalf("insert student %s: %v", number, err)
		}
		return id
	}
	createDevice := func(t *testing.T, name, identifier, deviceType string) (string, string) {
		t.Helper()
		rec := env.request(t, http.MethodPost, "/api/v1/devices", map[string]any{
			"name": name, "identifier": identifier, "device_type": deviceType,
		}, data.adminToken)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create device %s status = %d, body = %s", identifier, rec.Code, rec.Body.String())
		}
		var body deviceCreateResponse
		decode(t, rec, &body)
		id, _ := body.Device["id"].(string)
		if id == "" || body.IngestToken == "" {
			t.Fatalf("create device %s body = %s", identifier, rec.Body.String())
		}
		return id, body.IngestToken
	}

	readerID, readerToken := createDevice(t, "Reader Gerbang", "RFID-GATE-01", "rfid_reader")
	spareID, spareToken := createDevice(t, "Reader Cadangan", "RFID-GATE-02", "rfid_reader")
	studentID := insertStudent(t, "S-1001", "Budi Santoso")
	otherStudentID := insertStudent(t, "S-1002", "Siti Aminah")

	t.Run("card mapping", func(t *testing.T) {
		for name, token := range map[string]string{
			"teacher":   data.teacherToken,
			"principal": data.principalToken,
			"student":   data.studentToken,
		} {
			if rec := env.request(t, http.MethodPut, "/api/v1/students/"+studentID+"/card", map[string]any{
				"card_uid": "CARD-1001",
			}, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s card write status = %d", name, rec.Code)
			}
		}

		rec := env.request(t, http.MethodPut, "/api/v1/students/"+studentID+"/card", map[string]any{
			"card_uid": "  card-1001 ",
		}, data.staffTUToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("map card status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var mapped cardResponse
		decode(t, rec, &mapped)
		if mapped.StudentCard.StudentID != studentID || mapped.StudentCard.CardUID == nil || *mapped.StudentCard.CardUID != "CARD-1001" {
			t.Fatalf("mapped card = %+v", mapped.StudentCard)
		}

		// The same card cannot be mapped to a second student.
		if rec := env.request(t, http.MethodPut, "/api/v1/students/"+otherStudentID+"/card", map[string]any{
			"card_uid": "CARD-1001",
		}, data.adminToken); rec.Code != http.StatusConflict {
			t.Fatalf("duplicate card status = %d, body = %s", rec.Code, rec.Body.String())
		}
		for name, body := range map[string]map[string]any{
			"card with space": {"card_uid": "CARD 1002"},
			"oversize card":   {"card_uid": strings.Repeat("A", 65)},
		} {
			if rec := env.request(t, http.MethodPut, "/api/v1/students/"+otherStudentID+"/card", body, data.adminToken); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d", name, rec.Code)
			}
		}
		if rec := env.request(t, http.MethodPut, "/api/v1/students/00000000-0000-0000-0000-000000000000/card", map[string]any{
			"card_uid": "CARD-X",
		}, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown student card status = %d", rec.Code)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/students/"+studentID+"/card", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("get card status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &mapped)
		if mapped.StudentCard.CardUID == nil || *mapped.StudentCard.CardUID != "CARD-1001" {
			t.Fatalf("get card = %+v", mapped.StudentCard)
		}
		if rec := env.request(t, http.MethodGet, "/api/v1/students/"+studentID+"/card", nil, data.teacherToken); rec.Code != http.StatusForbidden {
			t.Fatalf("teacher get card status = %d", rec.Code)
		}
	})

	t.Run("device token auth and validation", func(t *testing.T) {
		batch := map[string]any{
			"device_identifier": "RFID-GATE-01",
			"taps": []map[string]any{
				{"card_uid": "CARD-1001", "occurred_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)},
			},
		}
		if rec := env.ingest(t, "", batch); rec.Code != http.StatusUnauthorized {
			t.Fatalf("missing token status = %d", rec.Code)
		}
		if rec := env.ingest(t, "dt_not-a-real-token", batch); rec.Code != http.StatusUnauthorized {
			t.Fatalf("bogus token status = %d", rec.Code)
		}
		if rec := env.ingest(t, readerToken, map[string]any{
			"device_identifier": "RFID-OTHER",
			"taps":              batch["taps"],
		}); rec.Code != http.StatusUnauthorized {
			t.Fatalf("identifier mismatch status = %d, body = %s", rec.Code, rec.Body.String())
		}

		manyTaps := make([]map[string]any, 101)
		for i := range manyTaps {
			manyTaps[i] = map[string]any{"card_uid": "CARD-1001", "occurred_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}
		}
		for name, invalid := range map[string]map[string]any{
			"empty batch":     {"device_identifier": "RFID-GATE-01", "taps": []map[string]any{}},
			"oversize batch":  {"device_identifier": "RFID-GATE-01", "taps": manyTaps},
			"blank card":      {"device_identifier": "RFID-GATE-01", "taps": []map[string]any{{"card_uid": " ", "occurred_at": time.Now().UTC().Format(time.RFC3339)}}},
			"card with space": {"device_identifier": "RFID-GATE-01", "taps": []map[string]any{{"card_uid": "CARD 1", "occurred_at": time.Now().UTC().Format(time.RFC3339)}}},
			"future tap":      {"device_identifier": "RFID-GATE-01", "taps": []map[string]any{{"card_uid": "CARD-1001", "occurred_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}}},
			"missing time":    {"device_identifier": "RFID-GATE-01", "taps": []map[string]any{{"card_uid": "CARD-1001"}}},
		} {
			if rec := env.ingest(t, readerToken, invalid); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}

		// Inactive devices stop accepting taps without losing the token.
		if rec := env.request(t, http.MethodPatch, "/api/v1/devices/"+spareID+"/status", map[string]any{
			"status": "inactive",
		}, data.adminToken); rec.Code != http.StatusOK {
			t.Fatalf("deactivate spare status = %d", rec.Code)
		}
		if rec := env.ingest(t, spareToken, map[string]any{
			"device_identifier": "RFID-GATE-02",
			"taps":              batch["taps"],
		}); rec.Code != http.StatusForbidden {
			t.Fatalf("inactive device status = %d, body = %s", rec.Code, rec.Body.String())
		}
	})

	occurred := time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Second)
	t.Run("ingest, idempotency and last seen", func(t *testing.T) {
		batch := map[string]any{
			"device_identifier": "rfid-gate-01",
			"taps": []map[string]any{
				{"card_uid": "card-1001", "occurred_at": occurred.Format(time.RFC3339)},
				{"card_uid": "CARD-9999", "occurred_at": occurred.Format(time.RFC3339)},
			},
		}
		rec := env.ingest(t, readerToken, batch)
		if rec.Code != http.StatusOK {
			t.Fatalf("ingest status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var result tapIngestResponse
		decode(t, rec, &result)
		if result.Accepted != 2 || result.Duplicates != 0 || result.Unknown != 1 {
			t.Fatalf("ingest result = %+v", result)
		}

		rec = env.ingest(t, readerToken, batch)
		if rec.Code != http.StatusOK {
			t.Fatalf("replay status = %d, body = %s", rec.Code, rec.Body.String())
		}
		decode(t, rec, &result)
		if result.Accepted != 0 || result.Duplicates != 2 || result.Unknown != 0 {
			t.Fatalf("replay result = %+v", result)
		}

		rec = env.request(t, http.MethodGet, "/api/v1/devices/"+readerID, nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("get reader status = %d", rec.Code)
		}
		var getBody struct {
			Device map[string]any `json:"device"`
		}
		decode(t, rec, &getBody)
		if getBody.Device["last_seen_at"] == nil {
			t.Fatalf("last_seen_at not set: %s", rec.Body.String())
		}
	})

	t.Run("token rotation", func(t *testing.T) {
		rec := env.request(t, http.MethodPost, "/api/v1/devices/"+readerID+"/token", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("rotate status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var rotated struct {
			IngestToken string `json:"ingest_token"`
		}
		decode(t, rec, &rotated)
		if rotated.IngestToken == "" || rotated.IngestToken == readerToken {
			t.Fatalf("rotated token = %q", rotated.IngestToken)
		}
		batch := map[string]any{
			"device_identifier": "RFID-GATE-01",
			"taps": []map[string]any{
				{"card_uid": "CARD-1001", "occurred_at": occurred.Add(time.Minute).Format(time.RFC3339)},
			},
		}
		if rec := env.ingest(t, readerToken, batch); rec.Code != http.StatusUnauthorized {
			t.Fatalf("old token status = %d", rec.Code)
		}
		if rec := env.ingest(t, rotated.IngestToken, batch); rec.Code != http.StatusOK {
			t.Fatalf("new token status = %d, body = %s", rec.Code, rec.Body.String())
		}
		if rec := env.request(t, http.MethodPost, "/api/v1/devices/00000000-0000-0000-0000-000000000000/token", nil, data.adminToken); rec.Code != http.StatusNotFound {
			t.Fatalf("rotate unknown status = %d", rec.Code)
		}
	})

	t.Run("tap log read and filters", func(t *testing.T) {
		for name, token := range map[string]string{
			"teacher":  data.teacherToken,
			"staff_tu": data.staffTUToken,
			"student":  data.studentToken,
		} {
			if rec := env.request(t, http.MethodGet, "/api/v1/device-taps", nil, token); rec.Code != http.StatusForbidden {
				t.Fatalf("%s tap list status = %d", name, rec.Code)
			}
		}
		for name, token := range map[string]string{
			"admin":     data.adminToken,
			"duty":      dutyToken,
			"principal": data.principalToken,
		} {
			rec := env.request(t, http.MethodGet, "/api/v1/device-taps", nil, token)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s tap list status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
			var list tapListResponse
			decode(t, rec, &list)
			if list.Total != 3 || len(list.Data) != 3 {
				t.Fatalf("%s tap list = %+v", name, list)
			}
		}

		rec := env.request(t, http.MethodGet, "/api/v1/device-taps?card_uid=card-9999&limit=1", nil, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("card filter status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var list tapListResponse
		decode(t, rec, &list)
		if list.Total != 1 || len(list.Data) != 1 || list.Data[0]["student_id"] != nil || list.Limit != 1 {
			t.Fatalf("card filter = %+v", list)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/device-taps?student_id="+studentID, nil, data.adminToken)
		decode(t, rec, &list)
		if list.Total != 2 {
			t.Fatalf("student filter total = %d", list.Total)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/device-taps?device_id="+readerID+"&from="+occurred.Format("2006-01-02")+"&to="+occurred.Format("2006-01-02"), nil, data.adminToken)
		decode(t, rec, &list)
		if list.Total != 3 {
			t.Fatalf("date filter total = %d", list.Total)
		}
		for name, query := range map[string]string{
			"bad device id": "device_id=not-a-uuid",
			"bad date":      "from=2026-13-01",
			"bad limit":     "limit=abc",
			"oversize page": "limit=500",
			"reversed date": "from=2026-09-02&to=2026-09-01",
		} {
			if rec := env.request(t, http.MethodGet, "/api/v1/device-taps?"+query, nil, data.adminToken); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s status = %d, body = %s", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("card can be cleared", func(t *testing.T) {
		rec := env.request(t, http.MethodPut, "/api/v1/students/"+studentID+"/card", map[string]any{
			"card_uid": nil,
		}, data.adminToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("clear card status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var cleared cardResponse
		decode(t, rec, &cleared)
		if cleared.StudentCard.CardUID != nil {
			t.Fatalf("cleared card = %+v", cleared.StudentCard)
		}
		rec = env.request(t, http.MethodGet, "/api/v1/students/"+studentID+"/card", nil, data.adminToken)
		decode(t, rec, &cleared)
		if cleared.StudentCard.CardUID != nil {
			t.Fatalf("get cleared card = %+v", cleared.StudentCard)
		}
	})

	t.Run("writes are audited", func(t *testing.T) {
		rows, err := env.pool.Query(ctx,
			`SELECT action FROM audit_logs
			 WHERE (entity_type = 'device' AND action = 'device.token')
			    OR (entity_type = 'student' AND action = 'student.card')
			 ORDER BY created_at, id`)
		if err != nil {
			t.Fatalf("query audit logs: %v", err)
		}
		defer rows.Close()
		var actions []string
		for rows.Next() {
			var action string
			if err := rows.Scan(&action); err != nil {
				t.Fatalf("scan audit row: %v", err)
			}
			actions = append(actions, action)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate audit rows: %v", err)
		}
		want := []string{"student.card", "device.token", "student.card"}
		if len(actions) != len(want) {
			t.Fatalf("audit actions = %v, want %v", actions, want)
		}
		for i := range want {
			if actions[i] != want[i] {
				t.Fatalf("audit actions = %v, want %v", actions, want)
			}
		}
	})
}

func (e apiEnv) ingest(t *testing.T, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal ingest body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/device-taps", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Device-Token", token)
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}
