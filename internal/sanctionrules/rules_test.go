package sanctionrules

import (
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ghalitsar-coder/backend-ifter/internal/db"
)

func intPtr(value int) *int { return &value }

func baseInput() RuleInput {
	return RuleInput{
		Name:      "Peringatan tertulis",
		MinPoints: intPtr(10),
		Action:    "Suratan peringatan",
		Level:     "sedang",
	}
}

func TestNormalizeRuleInput(t *testing.T) {
	t.Run("valid open-ended rule", func(t *testing.T) {
		n, err := normalizeRuleInput(baseInput())
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if n.minPoints != 10 || n.maxPoints.Valid || n.rng.max != openEndedMax || n.rng.min != 10 {
			t.Fatalf("normalized = %+v", n)
		}
		if !n.active {
			t.Fatal("default is_active should be true")
		}
	})
	t.Run("bounded rule keeps inclusive max", func(t *testing.T) {
		in := baseInput()
		in.MaxPoints = intPtr(19)
		n, err := normalizeRuleInput(in)
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if !n.maxPoints.Valid || n.maxPoints.Int16 != 19 || n.rng.max != 19 {
			t.Fatalf("normalized = %+v", n)
		}
	})
	t.Run("missing name derives from action", func(t *testing.T) {
		in := baseInput()
		in.Name = "  "
		n, err := normalizeRuleInput(in)
		if err != nil {
			t.Fatalf("normalize: %v", err)
		}
		if n.name != in.Action {
			t.Fatalf("derived name = %q, want %q", n.name, in.Action)
		}
	})
	t.Run("invalid payloads", func(t *testing.T) {
		invalid := []struct {
			name   string
			mutate func(*RuleInput)
		}{
			{"missing min_points", func(in *RuleInput) { in.MinPoints = nil }},
			{"negative min", func(in *RuleInput) { *in.MinPoints = -1 }},
			{"min above smallint", func(in *RuleInput) { *in.MinPoints = math.MaxInt16 + 1 }},
			{"max below min", func(in *RuleInput) { in.MaxPoints = intPtr(9) }},
			{"max equals min rejected (strict CHECK)", func(in *RuleInput) { in.MaxPoints = intPtr(10) }},
			{"negative max", func(in *RuleInput) { in.MaxPoints = intPtr(-2) }},
			{"missing action", func(in *RuleInput) { in.Action = "   " }},
			{"oversized name", func(in *RuleInput) { in.Name = string(make([]byte, 101)) }},
			{"oversized level", func(in *RuleInput) { in.Level = string(make([]byte, 31)) }},
			{"NUL in description", func(in *RuleInput) { in.Description = "a\x00b" }},
		}
		for _, tt := range invalid {
			t.Run(tt.name, func(t *testing.T) {
				in := baseInput()
				tt.mutate(&in)
				if _, err := normalizeRuleInput(in); err != ErrInvalidInput {
					t.Fatalf("err = %v, want ErrInvalidInput", err)
				}
			})
		}
	})
}

func TestRuleRangeOverlaps(t *testing.T) {
	cases := []struct {
		a, b    ruleRange
		overlap bool
	}{
		{ruleRange{0, 9}, ruleRange{10, 19}, false},
		{ruleRange{0, 10}, ruleRange{10, 19}, true}, // inclusive bounds touch at 10
		{ruleRange{5, 9}, ruleRange{0, 100}, true},
		{ruleRange{20, openEndedMax}, ruleRange{10, 20}, true},
		{ruleRange{21, openEndedMax}, ruleRange{10, 20}, false},
		{ruleRange{0, openEndedMax}, ruleRange{0, 5}, true},
	}
	for _, tt := range cases {
		if got := tt.a.overlaps(tt.b); got != tt.overlap {
			t.Errorf("overlap(%+v,%+v) = %v, want %v", tt.a, tt.b, got, tt.overlap)
		}
		if got := tt.b.overlaps(tt.a); got != tt.overlap {
			t.Errorf("overlap is not symmetric for %+v/%+v", tt.a, tt.b)
		}
	}
}

func TestRangeOverlapsActive(t *testing.T) {
	rows := []db.ListActiveSanctionRuleRangesForUpdateRow{
		{ID: uuidFixture(1), MinPoints: 0, MaxPoints: int2Fixture(9)},
		{ID: uuidFixture(2), MinPoints: 10, MaxPoints: int2Fixture(19)},
		{ID: uuidFixture(3), MinPoints: 20, MaxPoints: int2Fixture(25)},
	}
	if !rangeOverlapsActive(rows, ruleRange{min: 5, max: 12}, pgtype.UUID{}) {
		t.Fatal("candidate 5..12 overlaps [0,9] and [10,19]")
	}
	if rangeOverlapsActive(rows, ruleRange{min: 26, max: openEndedMax}, pgtype.UUID{}) {
		t.Fatal("candidate above every range must not overlap")
	}
	// Excluding the colliding row's own id allows a self-update.
	collide := ruleRange{min: 0, max: 3}
	if !rangeOverlapsActive(rows, collide, pgtype.UUID{}) {
		t.Fatal("candidate 0..3 overlaps [0,9]")
	}
	if rangeOverlapsActive(rows, collide, uuidFixture(1)) {
		t.Fatal("excluding the overlapping row should allow the update")
	}
}

func uuidFixture(seed int) pgtype.UUID {
	var b [16]byte
	b[0] = byte(seed)
	b[15] = byte(seed + 1)
	return pgtype.UUID{Bytes: b, Valid: true}
}

func int2Fixture(value int16) pgtype.Int2 {
	return pgtype.Int2{Int16: value, Valid: true}
}
