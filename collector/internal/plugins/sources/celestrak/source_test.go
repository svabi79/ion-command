package celestrak

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// testdata/amateur_live.tle is the CelesTrak amateur group captured live on
// 2026-07-19 (93 objects, starting with OSCAR 7).
func TestParseLiveTLESet(t *testing.T) {
	body, err := os.ReadFile("testdata/amateur_live.tle")
	if err != nil {
		t.Fatal(err)
	}
	sets := ParseTLESets(body)
	if len(sets) < 80 {
		t.Fatalf("expected the full amateur group, got %d", len(sets))
	}
	if sets[0].name != "OSCAR 7 (AO-7)" || sets[0].norad != "07530" {
		t.Fatalf("unexpected first object: %+v", sets[0])
	}
}

func TestPositionRecordPropagatesPlausibly(t *testing.T) {
	body, err := os.ReadFile("testdata/amateur_live.tle")
	if err != nil {
		t.Fatal(err)
	}
	sets := ParseTLESets(body)
	// AO-7 flies a ~1,450 km sun-synchronous-ish orbit at 101.99 degrees
	// inclination; propagate close to the TLE epoch (2026 day 200).
	at := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	record, ok := PositionRecord(sets[0], at, "test")
	if !ok {
		t.Fatal("propagation rejected a healthy TLE")
	}
	var payload map[string]any
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	alt := payload["altKm"].(float64)
	if alt < 1300 || alt > 1600 {
		t.Fatalf("AO-7 altitude implausible: %v km", alt)
	}
	lat := payload["latitude"].(float64)
	lon := payload["longitude"].(float64)
	if lat < -102 || lat > 102 || lon < -180 || lon > 180 {
		t.Fatalf("position out of range: %v/%v", lat, lon)
	}
	if record.Domain != "orbital" {
		t.Fatalf("unexpected domain %q", record.Domain)
	}
}
