package orbital

import (
	"math"
	"testing"
)

func TestVisibilityRadiusISS(t *testing.T) {
	radius, ok := visibilityRadiusDeg(420)
	if !ok {
		t.Fatal("ISS altitude should produce a footprint")
	}
	// Geometric horizon at ~420 km is about 20°.
	if radius < 18 || radius > 23 {
		t.Fatalf("unexpected ISS horizon radius %.2f deg", radius)
	}
}

func TestVisibilityRingClosedAndBounded(t *testing.T) {
	ring, ok := visibilityRing(47.5, 8.5, 800, 24)
	if !ok {
		t.Fatal("expected a ring")
	}
	if len(ring) != 25 {
		t.Fatalf("expected 24 vertices plus close, got %d", len(ring))
	}
	if ring[0][0] != ring[len(ring)-1][0] || ring[0][1] != ring[len(ring)-1][1] {
		t.Fatal("ring must be closed")
	}
	for i, pos := range ring {
		if pos[0] < -180 || pos[0] > 180 || pos[1] < -90 || pos[1] > 90 {
			t.Fatalf("vertex %d out of WGS84: %v", i, pos)
		}
	}
}

func TestVisibilityRingRejectsJunkAltitude(t *testing.T) {
	if _, ok := visibilityRing(0, 0, 50, 24); ok {
		t.Fatal("sub-orbital altitude must be rejected")
	}
	if _, ok := visibilityRing(0, 0, 80000, 24); ok {
		t.Fatal("beyond-GEO junk must be rejected")
	}
}

func TestDestinationAntipodeSafe(t *testing.T) {
	lat, lon := destination(0, 0, 90, 90)
	if math.Abs(lat) > 1e-9 || math.Abs(lon-90) > 1e-6 {
		t.Fatalf("90° east of origin: got %v, %v", lat, lon)
	}
}
