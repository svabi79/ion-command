package solar

import (
	"math"
	"testing"
	"time"
)

func TestSubsolarEquinoxNearEquator(t *testing.T) {
	at := time.Date(2026, 3, 20, 15, 6, 0, 0, time.UTC)
	lat, lon := Subsolar(at.Unix(), at.Nanosecond())
	if math.Abs(lat) > 2.5 {
		t.Fatalf("equinox subsolar latitude should be near 0, got %.2f (lon %.2f)", lat, lon)
	}
	if lon < -180 || lon > 180 {
		t.Fatalf("subsolar longitude out of range: %.2f", lon)
	}
}

func TestTwilightBandClosedAndBounded(t *testing.T) {
	ring := TwilightBand(0, 0)
	if len(ring) < 20 {
		t.Fatalf("expected a usable band, got %d vertices", len(ring))
	}
	if ring[0][0] != ring[len(ring)-1][0] || ring[0][1] != ring[len(ring)-1][1] {
		t.Fatal("band ring must be closed")
	}
	for i, pos := range ring {
		if pos[0] < -180 || pos[0] > 180 || pos[1] < -90 || pos[1] > 90 {
			t.Fatalf("vertex %d out of WGS84: %v", i, pos)
		}
	}
}
