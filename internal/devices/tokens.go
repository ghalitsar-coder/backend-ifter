package devices

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// newIngestToken returns the plaintext bearer token (shown once) and the
// SHA-256 hex digest stored on the device row.
func newIngestToken() (token, hash string, err error) {
	buf := make([]byte, ingestTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate ingest token: %w", err)
	}
	token = ingestTokenPfx + hex.EncodeToString(buf)
	return token, hashIngestToken(token), nil
}

func hashIngestToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
