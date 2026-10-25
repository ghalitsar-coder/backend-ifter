package teacherpresence

import (
	"math"
	"testing"
	"time"
)

func almostEqual(t *testing.T, got, want, tolerance float64, label string) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("%s = %.3f, want %.3f (±%.3f)", label, got, want, tolerance)
	}
}

func TestHaversineIdenticalPoints(t *testing.T) {
	if got := HaversineMeters(-6.2, 106.8, -6.2, 106.8); got != 0 {
		t.Fatalf("distance for identical points = %v, want 0", got)
	}
}

func TestHaversineOneDegreeLatitude(t *testing.T) {
	// One meridian degree is pi/180 * R regardless of longitude at the sphere.
	want := math.Pi / 180 * earthRadiusMeters
	got := HaversineMeters(0, 0, 1, 0)
	almostEqual(t, got, want, want*0.001, "one degree latitude")
}

func TestHaversineSmallOffset(t *testing.T) {
	// 0.001 degrees of latitude ~ 111.19 m; used by the handler tests' fixes.
	got := HaversineMeters(-6.200, 106.816666, -6.201, 106.816666)
	almostEqual(t, got, 111.195, 0.5, "0.001 degree latitude")
}

func TestHaversineJakartaToBandung(t *testing.T) {
	// Monas (-6.17539, 106.82715) to Alun-alun Bandung (-6.99307, 107.60533)
	// is about 125 km great-circle.
	got := HaversineMeters(-6.17539, 106.82715, -6.99307, 107.60533)
	if got < 120000 || got > 130000 {
		t.Fatalf("Jakarta-Bandung distance = %v, want within 120-130 km", got)
	}
}

func TestHaversineSymmetricAndLongitudeAtEquator(t *testing.T) {
	forward := HaversineMeters(12.34, -56.78, -87.65, 43.21)
	backward := HaversineMeters(-87.65, 43.21, 12.34, -56.78)
	if forward != backward {
		t.Fatalf("haversine not symmetric: %v vs %v", forward, backward)
	}
	// One equator degree of longitude ~ 111.195 km as well.
	almostEqual(t, HaversineMeters(0, 0, 0, 1), math.Pi/180*earthRadiusMeters, 1, "one degree longitude at equator")
}

func TestWithinGeofence(t *testing.T) {
	if !withinGeofence(false, 999999, 1) {
		t.Fatal("disabled geofence must always pass")
	}
	if !withinGeofence(true, 299.9, 300) {
		t.Fatal("distance below radius must be inside")
	}
	if !withinGeofence(true, 300, 300) {
		t.Fatal("distance equal to radius must be inside")
	}
	if withinGeofence(true, 300.01, 300) {
		t.Fatal("distance above radius must be outside")
	}
}

func TestDurationMinutesAndRound(t *testing.T) {
	base := time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)
	if got := durationMinutesBetween(base, base.Add(90*time.Minute)); got != 90 {
		t.Fatalf("duration = %v, want 90", got)
	}
	if got := durationMinutesBetween(base.Add(time.Hour), base); got != 0 {
		t.Fatalf("negative duration = %v, want 0", got)
	}
	if got := durationMinutesBetween(base, time.Time{}); got != 0 {
		t.Fatalf("zero end duration = %v, want 0", got)
	}
	if got := round2(123.456); got != 123.46 {
		t.Fatalf("round2 = %v, want 123.46", got)
	}
}
