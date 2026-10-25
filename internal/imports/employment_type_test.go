package imports

import "testing"

func TestNormalizeEmploymentType(t *testing.T) {
	cases := []struct {
		value     string
		wantCode  string
		wantKnown bool
	}{
		{"", "", true},
		{"   ", "", true},
		{"Permanent", "permanent", true},
		{"permanent", "permanent", true},
		{" PERMANEN ", "permanent", true},
		{"Honorer", "honorary", true},
		{"honorary", "honorary", true},
		{"Staf TU", "staff_tu", true},
		{"staf tu", "staff_tu", true},
		{"Staff TU", "staff_tu", true},
		{"staff_tu", "staff_tu", true},
		{"Kontrak", "", false},
		{"PNS", "", false},
		{"honour", "", false},
	}
	for _, tt := range cases {
		code, known := normalizeEmploymentType(tt.value)
		if code != tt.wantCode || known != tt.wantKnown {
			t.Errorf("normalizeEmploymentType(%q) = %q/%v, want %q/%v", tt.value, code, known, tt.wantCode, tt.wantKnown)
		}
	}
}

func TestOptionalColumn(t *testing.T) {
	header := []string{"NIP", "Nama Lengkap", "Email", "Telepon", "Status", "Jenis Kepegawaian"}
	if index := optionalColumn(header, "Jenis Kepegawaian"); index != 5 {
		t.Fatalf("optionalColumn found %d, want 5", index)
	}
	if index := optionalColumn(header, "jenis kepegawaian"); index != 5 {
		t.Fatalf("optionalColumn must be case-insensitive, got %d", index)
	}
	if index := optionalColumn([]string{"NIP", "Nama Lengkap"}, "Jenis Kepegawaian"); index != -1 {
		t.Fatalf("optionalColumn on absent column = %d, want -1", index)
	}
}
