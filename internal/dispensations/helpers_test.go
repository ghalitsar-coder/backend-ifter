package dispensations

import (
	"strings"
	"testing"
)

func TestValidateDocument(t *testing.T) {
	pdf := append([]byte("%PDF-1.4 rest of a tiny pdf"), make([]byte, 64)...)
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3}
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, make([]byte, 32)...)

	tests := []struct {
		name        string
		filename    string
		contentType string
		data        []byte
		wantExt     string
		wantErr     error
	}{
		{"pdf accepted", "surat sakit.PDF", "application/pdf", pdf, ".pdf", nil},
		{"jpeg accepted", "foto.jpg", "image/jpeg", jpeg, ".jpg", nil},
		{"jpeg jpeg ext accepted", "foto.jpeg", "", jpeg, ".jpeg", nil},
		{"png accepted", "scan.png", "image/png; charset=binary", png, ".png", nil},
		{"html rejected", "evil.html", "text/html", []byte("<html>"), "", ErrInvalidInput},
		{"docx rejected", "surat.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", []byte("PK\x03\x04"), "", ErrInvalidInput},
		{"no extension rejected", "document", "application/pdf", pdf, "", ErrInvalidInput},
		{"mime mismatch rejected", "surat.pdf", "image/png", pdf, "", ErrInvalidInput},
		{"fake pdf magic rejected", "surat.pdf", "application/pdf", []byte("MZ not a pdf at all"), "", ErrInvalidInput},
		{"empty payload rejected", "surat.pdf", "application/pdf", nil, "", ErrInvalidInput},
		{"path traversal in name still bounded by extension", "../x.pdf", "application/pdf", pdf, ".pdf", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, err := validateDocument(tt.filename, tt.contentType, tt.data)
			if err != tt.wantErr {
				t.Fatalf("validateDocument(%q,%q) error = %v, want %v", tt.filename, tt.contentType, err, tt.wantErr)
			}
			if ext != tt.wantExt {
				t.Fatalf("extension = %q, want %q", ext, tt.wantExt)
			}
		})
	}
}

func TestContentTypeForExtension(t *testing.T) {
	if got := contentTypeForExtension(".pdf"); got != "application/pdf" {
		t.Fatalf("pdf content type = %q", got)
	}
	if got := contentTypeForExtension(".png"); got != "image/png" {
		t.Fatalf("png content type = %q", got)
	}
	if got := contentTypeForExtension(".bin"); got != "application/octet-stream" {
		t.Fatalf("unknown extension content type = %q", got)
	}
}

func TestEnumValidators(t *testing.T) {
	for _, value := range []string{"sick", "late", "permit", "other"} {
		if !validType(value) {
			t.Fatalf("validType(%q) = false", value)
		}
	}
	for _, value := range []string{"", "izin", "sakit", "SICK"} {
		if validType(value) {
			t.Fatalf("validType(%q) = true, want false", value)
		}
	}
	if validDecision("pending") {
		t.Fatal("pending must not be a verify decision")
	}
	if !validDecision("approved") || !validDecision("rejected") {
		t.Fatal("approved/rejected must be valid decisions")
	}
	if !validStatus("pending") || validStatus("verified") {
		t.Fatal("status domain must be pending|approved|rejected")
	}
}

func TestCleanText(t *testing.T) {
	if got, err := cleanText("  halo  ", 10); err != nil || got != "halo" {
		t.Fatalf("cleanText trim = %q, %v", got, err)
	}
	if _, err := cleanText("bad\x00text", 10); err != ErrInvalidInput {
		t.Fatalf("NUL text error = %v, want ErrInvalidInput", err)
	}
	if _, err := cleanText(strings.Repeat("a", 1001), 1000); err != ErrInvalidInput {
		t.Fatalf("oversize text error = %v, want ErrInvalidInput", err)
	}
}

func TestNormalizePage(t *testing.T) {
	if limit, offset := normalizePage(0, -5); limit != 50 || offset != 0 {
		t.Fatalf("normalizePage(0,-5) = %d,%d", limit, offset)
	}
	if limit, _ := normalizePage(500, 0); limit != 100 {
		t.Fatalf("limit cap = %d, want 100", limit)
	}
}

func TestConfigFromEnvironment(t *testing.T) {
	t.Setenv("IFTER_IMPORTS_DIR", "/tmp/disp-store")
	t.Setenv("IFTER_IMPORTS_MAX_FILE_SIZE", "2048")
	cfg := ConfigFromEnvironment()
	if cfg.Dir != "/tmp/disp-store" || cfg.MaxFileSize != 2048 {
		t.Fatalf("ConfigFromEnvironment() = %+v", cfg)
	}
	t.Setenv("IFTER_IMPORTS_MAX_FILE_SIZE", "garbage")
	cfg = ConfigFromEnvironment()
	if cfg.MaxFileSize != 5<<20 {
		t.Fatalf("invalid env must fall back to default, got %d", cfg.MaxFileSize)
	}
}

func TestNewServiceDefaults(t *testing.T) {
	s := NewService(nil, nil, Config{})
	if s.MaxFileSize() != 5<<20 {
		t.Fatalf("default max size = %d", s.MaxFileSize())
	}
	if !strings.HasSuffix(s.dir, "storage/imports") {
		t.Fatalf("default dir = %q", s.dir)
	}
}
