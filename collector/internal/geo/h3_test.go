package geo

import "testing"

func TestParseAndCenterInBounds(t *testing.T) {
	index, ok := ParseH3("84754e7ffffffff")
	if !ok {
		t.Fatal("parse")
	}
	lat, lon, ok := CellToLatLng(index)
	if !ok || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		t.Fatalf("center %v %v %v", lat, lon, ok)
	}
}
