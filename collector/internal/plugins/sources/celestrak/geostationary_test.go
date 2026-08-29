package celestrak

import (
	"math"
	"testing"
	"time"
)

// A geostationary satellite is the one case where the look angle can be
// checked against a closed-form answer instead of against this code's own
// output. ES'HAIL 2 (QO-100) holds station at 25.9 E, and the elevation and
// azimuth from any ground point follow from spherical geometry alone:
//
//	el = atan((cos(dLon)cos(lat) - Re/r) / sqrt(1 - cos^2(dLon)cos^2(lat)))
//	az = 180 + atan(tan(dLon) / sin(lat))     (northern hemisphere)
//
// where Re/r = 6378/42164 = 0.15127, and dLon is the station longitude minus
// the sub-satellite longitude - not the other way round, or a satellite to
// the east comes out bearing west of south.
//
// If SGP4, the ECI-to-topocentric conversion or the degree/radian handling
// were wrong, the two would not agree - which is what makes this worth more
// than "the number looks plausible".
func TestGeostationaryLookAngleMatchesClosedForm(t *testing.T) {
	sets := fixtureSets(t)
	var qo100 trackedSatellite
	found := false
	for _, tracked := range sets {
		if tracked.name == "ES'HAIL 2" {
			qo100, found = tracked, true
			break
		}
	}
	if !found {
		t.Skip("fixture has no geostationary satellite to check against")
	}

	const (
		stationLatitude  = 47.52 // JN47om
		stationLongitude = 9.21
		subSatLongitude  = 25.9 // ES'HAIL 2 station keeping slot
		earthOverRadius  = 6378.0 / 42164.0
	)
	at := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	station := observer{latitude: stationLatitude, longitude: stationLongitude, set: true}

	elevation, azimuth, ok := station.elevationAt(qo100, at)
	if !ok {
		t.Fatal("propagation failed for the geostationary satellite")
	}

	radians := math.Pi / 180.0
	deltaLon := (stationLongitude - subSatLongitude) * radians
	latitude := stationLatitude * radians
	cosProduct := math.Cos(deltaLon) * math.Cos(latitude)
	expectedElevation := math.Atan((cosProduct-earthOverRadius)/math.Sqrt(1-cosProduct*cosProduct)) / radians
	expectedAzimuth := 180.0 + math.Atan(math.Tan(deltaLon)/math.Sin(latitude))/radians

	// One degree of tolerance: the satellite drifts slightly within its
	// station-keeping box, and the closed form assumes a perfect sphere.
	if math.Abs(elevation-expectedElevation) > 1.0 {
		t.Fatalf("elevation %.2f deg disagrees with the closed form %.2f deg", elevation, expectedElevation)
	}
	if math.Abs(azimuth-expectedAzimuth) > 1.5 {
		t.Fatalf("azimuth %.2f deg disagrees with the closed form %.2f deg", azimuth, expectedAzimuth)
	}

	// It is geostationary, so an hour later it must still be in the same
	// place. A satellite that drifts here would mean the Earth rotation term
	// is being applied twice, or not at all.
	laterElevation, laterAzimuth, ok := station.elevationAt(qo100, at.Add(time.Hour))
	if !ok {
		t.Fatal("propagation failed an hour later")
	}
	if math.Abs(laterElevation-elevation) > 0.5 || math.Abs(laterAzimuth-azimuth) > 0.5 {
		t.Fatalf("a geostationary satellite moved: %.2f/%.2f -> %.2f/%.2f in one hour",
			elevation, azimuth, laterElevation, laterAzimuth)
	}
}
