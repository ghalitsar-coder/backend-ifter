package correctionevidence

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// The contract: sha256 over the JSON of
// {old_status,new_status,check_in_at,notes,reason,corrected_by,corrected_at}
// with keys in exactly that order, no whitespace, absent values as null.
func TestCanonicalJSONExactLayout(t *testing.T) {
	checkIn := "2026-09-02T00:10:00Z"
	notes := "dikoreksi"
	c := CanonicalCorrection{
		OldStatus:   "present",
		NewStatus:   "late",
		CheckInAt:   &checkIn,
		Notes:       &notes,
		Reason:      "salah input",
		CorrectedBy: "11111111-2222-3333-4444-555555555555",
		CorrectedAt: "2026-09-02T02:00:00Z",
	}
	want := `{"old_status":"present","new_status":"late","check_in_at":"2026-09-02T00:10:00Z","notes":"dikoreksi","reason":"salah input","corrected_by":"11111111-2222-3333-4444-555555555555","corrected_at":"2026-09-02T02:00:00Z"}`
	got := string(c.CanonicalJSON())
	if got != want {
		t.Fatalf("canonical JSON = %s, want %s", got, want)
	}

	// Absent check_in_at and notes encode as JSON null literals.
	nils := CanonicalCorrection{OldStatus: "a", NewStatus: "b", Reason: "r", CorrectedBy: "u", CorrectedAt: "t"}
	if got := string(nils.CanonicalJSON()); !strings.Contains(got, `"check_in_at":null`) || !strings.Contains(got, `"notes":null`) {
		t.Fatalf("canonical JSON nulls = %s", got)
	}
}

func TestHashCorrectionDeterministic(t *testing.T) {
	c := CanonicalCorrection{
		OldStatus:   "present",
		NewStatus:   "late",
		Reason:      "salah input",
		CorrectedBy: "11111111-2222-3333-4444-555555555555",
		CorrectedAt: "2026-09-02T02:00:00Z",
	}
	first := HashCorrection(c)
	second := HashCorrection(c)
	if first != second {
		t.Fatalf("hash not deterministic: %s vs %s", first, second)
	}
	if len(first) != 64 || strings.ToLower(first) != first {
		t.Fatalf("hash is not lowercase hex sha256: %q", first)
	}
	sum := sha256.Sum256(c.CanonicalJSON())
	if want := hex.EncodeToString(sum[:]); want != first {
		t.Fatalf("hash = %s, want sha256(canonical JSON) = %s", first, want)
	}

	changed := c
	changed.Notes = nil
	other := "2026-09-02T02:00:01Z"
	changed.CorrectedAt = other
	if HashCorrection(changed) == first {
		t.Fatal("payload change must change the hash")
	}
}

func TestHashCorrectionFieldsEncoding(t *testing.T) {
	// Same instant expressed with a +07:00 offset must hash identically:
	// the payload is normalized to UTC RFC3339.
	jakarta := time.FixedZone("WIB", 7*3600)
	at := time.Date(2026, 9, 2, 7, 10, 0, 0, jakarta)
	checkIn := pgtype.Timestamptz{Time: at, Valid: true}
	notes := pgtype.Text{String: "dikoreksi", Valid: true}
	utc := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)

	h1 := HashCorrectionFields("present", "late", checkIn, notes, "salah input", "u1", utc)
	h2 := HashCorrectionFields("present", "late", checkIn, notes, "salah input", "u1", utc)
	if h1 != h2 {
		t.Fatalf("field hashing not deterministic: %s vs %s", h1, h2)
	}

	want := HashCorrection(CanonicalCorrection{
		OldStatus:   "present",
		NewStatus:   "late",
		CheckInAt:   strPtr("2026-09-02T00:10:00Z"),
		Notes:       strPtr("dikoreksi"),
		Reason:      "salah input",
		CorrectedBy: "u1",
		CorrectedAt: "2026-09-02T00:00:00Z",
	})
	if h1 != want {
		t.Fatalf("hash = %s, want %s", h1, want)
	}

	// Invalid (SQL NULL) check_in_at/notes encode as JSON null.
	nulls := HashCorrectionFields("present", "present", pgtype.Timestamptz{}, pgtype.Text{}, "r", "u1", utc)
	if nulls == h1 {
		t.Fatal("null payload fields must change the hash")
	}
}

func strPtr(value string) *string { return &value }
