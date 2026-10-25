// Package correctionevidence implements the correction-evidence feature
// package (docs/API_ADDENDUM.md §6.6, migration 000014): private document
// attachments for attendance corrections plus the canonical sha256 integrity
// hash that the server signs onto every new attendance correction.
//
// The server is the source of truth for the hash: clients never supply one.
package correctionevidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// CanonicalCorrection is the exact payload the integrity hash covers
// (package spec: old_status, new_status, check_in_at, notes, reason,
// corrected_by, corrected_at as RFC3339). Field declaration order is the
// canonical JSON key order — json.Marshal encodes struct fields in declared
// order, so the byte stream is deterministic across runs and machines.
// Do not reorder these fields or add any without updating the hash contract.
type CanonicalCorrection struct {
	OldStatus   string  `json:"old_status"`
	NewStatus   string  `json:"new_status"`
	CheckInAt   *string `json:"check_in_at"`
	Notes       *string `json:"notes"`
	Reason      string  `json:"reason"`
	CorrectedBy string  `json:"corrected_by"`
	CorrectedAt string  `json:"corrected_at"`
}

// CanonicalJSON returns the deterministic JSON encoding of the payload.
// Absent timestamps/notes encode as the JSON null literal.
func (c CanonicalCorrection) CanonicalJSON() []byte {
	// json.Marshal on a struct is deterministic: fixed field order, no
	// insignificant whitespace, stable string escaping. The error is
	// unreachable for this all-string/pointer struct.
	data, err := json.Marshal(c)
	if err != nil {
		panic("correctionevidence: canonical correction JSON is not encodable")
	}
	return data
}

// HashCorrection returns the lowercase hex sha256 of the canonical JSON.
func HashCorrection(c CanonicalCorrection) string {
	sum := sha256.Sum256(c.CanonicalJSON())
	return hex.EncodeToString(sum[:])
}

// HashCorrectionFields is the entry point the correction flow uses: it fixes
// the payload encoding (timestamps as UTC RFC3339, absent values as JSON
// null) so every caller hashes exactly the same bytes. checkInAt/notes are
// the post-correction values stored on the row.
func HashCorrectionFields(oldStatus, newStatus string, checkInAt pgtype.Timestamptz, notes pgtype.Text, reason, correctedBy string, correctedAt time.Time) string {
	payload := CanonicalCorrection{
		OldStatus:   oldStatus,
		NewStatus:   newStatus,
		CheckInAt:   rfc3339Ptr(checkInAt),
		Notes:       textPtr(notes),
		Reason:      reason,
		CorrectedBy: correctedBy,
		CorrectedAt: correctedAt.UTC().Format(time.RFC3339),
	}
	return HashCorrection(payload)
}

// rfc3339Ptr encodes an absent timestamp as JSON null.
func rfc3339Ptr(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	text := value.Time.UTC().Format(time.RFC3339)
	return &text
}
