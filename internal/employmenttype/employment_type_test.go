package employmenttype

import "testing"

func boolPtr(value bool) *bool { return &value }

func TestNormalizeInput(t *testing.T) {
	cases := []struct {
		name        string
		in          TypeInput
		wantCode    string
		wantName    string
		wantDesc    string
		wantInvalid bool
	}{
		{"seed-shaped permanent", TypeInput{Code: "permanent", Name: "Permanen"}, "permanent", "Permanen", "", false},
		{"staff_tu underscore code", TypeInput{Code: "staff_tu", Name: "Staf TU"}, "staff_tu", "Staf TU", "", false},
		{"trims spaces", TypeInput{Code: " honorary ", Name: "  Honorer  "}, "honorary", "Honorer", "", false},
		{"description kept", TypeInput{Code: "contract", Name: "Kontrak", Description: " Guru kontrak "}, "contract", "Kontrak", "Guru kontrak", false},
		{"empty code rejected", TypeInput{Code: "", Name: "x"}, "", "", "", true},
		{"empty name rejected", TypeInput{Code: "x", Name: "   "}, "", "", "", true},
		{"uppercase code rejected (CHECK format)", TypeInput{Code: "Permanent", Name: "x"}, "", "", "", true},
		{"dash in code rejected", TypeInput{Code: "staff-tu", Name: "x"}, "", "", "", true},
		{"space in code rejected", TypeInput{Code: "staff tu", Name: "x"}, "", "", "", true},
		{"code too long", TypeInput{Code: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "x"}, "", "", "", true}, // 31 chars
		{"name too long", TypeInput{Code: "ok", Name: strRepeat("a", 101)}, "", "", "", true},
		{"description too long", TypeInput{Code: "ok", Name: "x", Description: strRepeat("a", 2001)}, "", "", "", true},
		{"NUL byte rejected", TypeInput{Code: "bad\x00code", Name: "x"}, "", "", "", true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			code, name, description, err := normalizeInput(tt.in)
			if tt.wantInvalid {
				if err != ErrInvalidInput {
					t.Fatalf("expected ErrInvalidInput, got code=%q name=%q err=%v", code, name, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != tt.wantCode || name != tt.wantName || description != tt.wantDesc {
				t.Fatalf("normalize = %q/%q/%q, want %q/%q/%q", code, name, description, tt.wantCode, tt.wantName, tt.wantDesc)
			}
		})
	}
}

func strRepeat(s string, count int) string {
	out := make([]byte, 0, len(s)*count)
	for i := 0; i < count; i++ {
		out = append(out, s...)
	}
	return string(out)
}
