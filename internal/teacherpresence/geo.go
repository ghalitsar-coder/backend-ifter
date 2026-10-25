package teacherpresence

import "math"

const earthRadiusMeters = 6371008.8

// HaversineMeters returns the great-circle distance in meters between two
// WGS84 coordinate pairs (degrees). Server-side geofence decisions use this
// only; a client verdict is never trusted (TECH_STACK.md §7).
func HaversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)
	a := sinLat*sinLat + math.Cos(lat1*rad)*math.Cos(lat2*rad)*sinLon*sinLon
	return earthRadiusMeters * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// withinGeofence reports whether the measured distance is inside the school
// radius. A disabled geofence always passes.
func withinGeofence(enabled bool, distanceMeters float64, radiusMeters int32) bool {
	return !enabled || distanceMeters <= float64(radiusMeters)
}
