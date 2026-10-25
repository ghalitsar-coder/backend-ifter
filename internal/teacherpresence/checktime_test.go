package teacherpresence

import (
	"errors"
	"testing"
	"time"
)

func TestResolveCheckTimeWindow(t *testing.T) {
	// Absent value falls back to server-now.
	if got, err := resolveCheckTime("  "); err != nil || got.IsZero() {
		t.Fatalf("empty resolveCheckTime = %v, %v", got, err)
	}
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"23 hours ago inside window", time.Now().Add(-23 * time.Hour).Format(time.RFC3339), true},
		{"25 hours ago rejected", time.Now().Add(-25 * time.Hour).Format(time.RFC3339), false},
		{"30 seconds ahead allowed", time.Now().Add(30 * time.Second).Format(time.RFC3339), true},
		{"2 minutes ahead rejected", time.Now().Add(2 * time.Minute).Format(time.RFC3339), false},
		{"1 hour ahead rejected", time.Now().Add(time.Hour).Format(time.RFC3339), false},
		{"not RFC3339", "2026-10-25 07:00", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveCheckTime(tt.value)
			if tt.want {
				if err != nil || got.IsZero() {
					t.Fatalf("resolveCheckTime(%q) = %v, %v, want accepted", tt.value, got, err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("resolveCheckTime(%q) err = %v, want ErrInvalidInput", tt.value, err)
			}
		})
	}
}
