package imports

import "testing"

func TestParseDateCell(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "iso", value: "2010-01-02", want: "2010-01-02"},
		{name: "indonesian", value: "02/01/2010", want: "2010-01-02"},
		{name: "empty", value: ""},
		{name: "year only is not a serial", value: "2010", wantErr: true},
		{name: "invalid", value: "not-a-date", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDateCell(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDateCell(%q) error = nil", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDateCell(%q) error = %v", tt.value, err)
			}
			if tt.want == "" {
				if got.Valid {
					t.Fatalf("parseDateCell(%q) = %v, want invalid", tt.value, got)
				}
				return
			}
			if !got.Valid || got.Time.Format("2006-01-02") != tt.want {
				t.Fatalf("parseDateCell(%q) = %v, want %s", tt.value, got, tt.want)
			}
		})
	}
}
