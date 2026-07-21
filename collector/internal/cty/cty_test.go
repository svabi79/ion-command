package cty

import "testing"

func TestLookupKnownEntities(t *testing.T) {
	cases := []struct {
		call     string
		name     string
		lat, lon float64
	}{
		{"HB9ABC", "Switzerland", 46.9, 8.2},
		{"K1ABC", "United States", 37.5, -91.9},
		{"G4EDG/P", "England", 52.8, -2.1},
		{"JA1XYZ", "Japan", 36.4, 138.4},
		{"PY2ABC", "Brazil", -10.4, -53.2},
	}
	for _, c := range cases {
		entity, ok := Lookup(c.call)
		if !ok {
			t.Fatalf("%s not resolved", c.call)
		}
		if entity.Name != c.name {
			t.Fatalf("%s resolved to %q, expected %q", c.call, entity.Name, c.name)
		}
		if entity.Latitude < c.lat-3 || entity.Latitude > c.lat+3 || entity.Longitude < c.lon-6 || entity.Longitude > c.lon+6 {
			t.Fatalf("%s centroid off: %+v", c.call, entity)
		}
	}
	if _, ok := Lookup(""); ok {
		t.Fatal("empty callsign must not resolve")
	}
}
