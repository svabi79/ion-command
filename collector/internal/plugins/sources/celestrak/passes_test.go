package celestrak

import (
	"math"
	"os"
	"testing"
	"time"
)

func fixtureSets(t *testing.T) []trackedSatellite {
	t.Helper()
	body, err := os.ReadFile("testdata/amateur_live.tle")
	if err != nil {
		t.Fatal(err)
	}
	sets := ParseTLESets(body)
	if len(sets) == 0 {
		t.Fatal("no TLEs in fixture")
	}
	return sets
}

// The prediction and the live look angle are two different code paths over the
// same geometry, so they are each other's check: at the predicted culmination
// the live elevation must be the predicted peak, and at acquisition and loss
// it must be at the horizon. If either path had the frame or the sign wrong,
// they would disagree.
func TestNextPassAgreesWithLiveLookAngles(t *testing.T) {
	sets := fixtureSets(t)
	// The operator's own station, so the fixture exercises a real mid-latitude
	// site rather than a convenient one.
	station := observer{latitude: 47.52, longitude: 9.21, set: true}
	from := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	pass, ok := station.nextPass(sets[0], from)
	if !ok {
		t.Fatal("no pass found for AO-7 over JN47om within 24 h; that orbit passes several times a day")
	}

	if !pass.acquisition.Before(pass.culmination) || !pass.culmination.Before(pass.loss) {
		t.Fatalf("pass is out of order: aos=%s tca=%s los=%s", pass.acquisition, pass.culmination, pass.loss)
	}
	if pass.acquisition.Before(from) {
		t.Fatalf("pass starts before the search did: %s < %s", pass.acquisition, from)
	}

	peak, _, ok := station.elevationAt(sets[0], pass.culmination)
	if !ok {
		t.Fatal("propagation failed at culmination")
	}
	if math.Abs(peak-pass.peakDegrees) > 0.5 {
		t.Fatalf("live elevation at culmination (%.2f) disagrees with the predicted peak (%.2f)", peak, pass.peakDegrees)
	}
	if pass.peakDegrees < passMinPeakElevation {
		t.Fatalf("a pass below the reporting threshold was returned: %.2f", pass.peakDegrees)
	}

	// The horizon crossings are found to within one fine step, so the
	// elevation there must be at the horizon rather than merely somewhere.
	for label, when := range map[string]time.Time{"acquisition": pass.acquisition, "loss": pass.loss} {
		elevation, _, ok := station.elevationAt(sets[0], when)
		if !ok {
			t.Fatalf("propagation failed at %s", label)
		}
		if math.Abs(elevation) > 1.0 {
			t.Fatalf("%s should sit on the horizon, elevation was %.2f deg", label, elevation)
		}
	}

	// AO-7 sits near 1450 km, so a pass runs long by low-orbit standards but
	// is still bounded by geometry - not hours.
	if pass.duration() < 2*time.Minute || pass.duration() > 40*time.Minute {
		t.Fatalf("implausible pass duration: %s", pass.duration())
	}
	for label, azimuth := range map[string]float64{"aos": pass.acquisitionAzimuth, "los": pass.lossAzimuth} {
		if azimuth < 0 || azimuth > 360 {
			t.Fatalf("%s azimuth outside the compass: %.1f", label, azimuth)
		}
	}
}

// Successive predictions must march forward. Asking again from just after a
// pass has set must not return the same pass, which is what would happen if
// the "already up when the search started" case were mishandled.
func TestNextPassAdvances(t *testing.T) {
	sets := fixtureSets(t)
	station := observer{latitude: 47.52, longitude: 9.21, set: true}
	from := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	first, ok := station.nextPass(sets[0], from)
	if !ok {
		t.Fatal("no first pass")
	}
	// Search again from inside the pass: the answer must be the *next* one,
	// not the one already in progress.
	midPass := first.acquisition.Add(first.duration() / 2)
	second, ok := station.nextPass(sets[0], midPass)
	if !ok {
		t.Fatal("no second pass")
	}
	if !second.acquisition.After(first.loss) {
		t.Fatalf("a search started mid-pass returned an overlapping pass: first los=%s, second aos=%s",
			first.loss, second.acquisition)
	}
}

// Without a configured station there is nothing to predict against, and
// guessing would put the operator at the origin of the coordinate system.
func TestNextPassRequiresAnObserver(t *testing.T) {
	sets := fixtureSets(t)
	if _, ok := (observer{}).nextPass(sets[0], time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)); ok {
		t.Fatal("predicted a pass without a configured observer")
	}
}
