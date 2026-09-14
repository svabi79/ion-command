package iso3166

import "testing"

func TestLookupAlpha3(t *testing.T) {
	lat, lon, name, ok := Lookup("usa")
	if !ok || name != "United States" || lat == 0 || lon == 0 {
		t.Fatalf("usa: %v %v %q %v", lat, lon, name, ok)
	}
}

func TestLookupAlpha2(t *testing.T) {
	_, _, name, ok := Lookup("DE")
	if !ok || name != "Germany" {
		t.Fatalf("de: %q %v", name, ok)
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, _, _, ok := Lookup("ZZ"); ok {
		t.Fatal("unknown code must fail closed")
	}
}
