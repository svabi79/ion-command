package celestrak

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"testing"
	"time"
)

// A satellite's own sub-point is the one observer position whose look angle is
// known without a second implementation to check against: straight up, 90
// degrees elevation. Standing on the exact opposite side of the Earth is the
// other end of the same certainty - far below the horizon. Between them these
// two pin down both the sign convention and the frame conversion, which is
// where a look-angle routine actually goes wrong.
func TestLookAnglesAtSubPointAndAntipode(t *testing.T) {
	body, err := os.ReadFile("testdata/amateur_live.tle")
	if err != nil {
		t.Fatal(err)
	}
	sets := ParseTLESets(body)
	if len(sets) == 0 {
		t.Fatal("no TLEs in fixture")
	}
	at := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

	// Where is it? Take the sub-point from the position record itself, so the
	// test cannot drift from the code that emits it.
	record, ok := PositionRecord(sets[0], at, "test")
	if !ok {
		t.Fatal("propagation rejected a healthy TLE")
	}
	var payload struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		AltKm     float64 `json:"altKm"`
	}
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}

	beneath := observer{latitude: payload.Latitude, longitude: payload.Longitude, set: true}
	under, err := recordLookAngles(sets[0], at, beneath)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(under.elevation-90.0) > 1.0 {
		t.Fatalf("observer directly beneath the satellite should see ~90 deg elevation, got %.2f", under.elevation)
	}
	// Slant range from directly underneath is the altitude, give or take the
	// Earth model.
	if math.Abs(under.slantRange-payload.AltKm) > 25.0 {
		t.Fatalf("slant range from beneath (%.0f km) should match altitude (%.0f km)", under.slantRange, payload.AltKm)
	}

	antipodeLongitude := payload.Longitude + 180.0
	if antipodeLongitude > 180.0 {
		antipodeLongitude -= 360.0
	}
	opposite := observer{latitude: -payload.Latitude, longitude: antipodeLongitude, set: true}
	across, err := recordLookAngles(sets[0], at, opposite)
	if err != nil {
		t.Fatal(err)
	}
	if across.elevation > -50.0 {
		t.Fatalf("observer on the far side of the Earth should be well below the horizon, got %.2f deg", across.elevation)
	}
}

// An unset observer must add nothing to the payload: a station that was never
// configured should not produce look angles from the Gulf of Guinea.
func TestPositionRecordWithoutObserverOmitsLookAngles(t *testing.T) {
	body, err := os.ReadFile("testdata/amateur_live.tle")
	if err != nil {
		t.Fatal(err)
	}
	sets := ParseTLESets(body)
	record, ok := PositionRecord(sets[0], time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC), "test")
	if !ok {
		t.Fatal("propagation rejected a healthy TLE")
	}
	var payload map[string]any
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"azDeg", "elDeg", "rangeKm"} {
		if _, present := payload[key]; present {
			t.Fatalf("payload carries %q without a configured observer", key)
		}
	}
}

type angles struct {
	azimuth    float64
	elevation  float64
	slantRange float64
}

func recordLookAngles(tracked trackedSatellite, at time.Time, from observer) (angles, error) {
	record, ok := positionRecordFrom(tracked, at, "test", from)
	if !ok {
		return angles{}, errPropagation
	}
	var payload struct {
		AzDeg   float64 `json:"azDeg"`
		ElDeg   float64 `json:"elDeg"`
		RangeKm float64 `json:"rangeKm"`
	}
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return angles{}, err
	}
	return angles{azimuth: payload.AzDeg, elevation: payload.ElDeg, slantRange: payload.RangeKm}, nil
}

var errPropagation = errors.New("propagation rejected the fixture TLE")
