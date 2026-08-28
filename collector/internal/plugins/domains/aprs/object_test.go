package aprs

import "testing"

func TestDecodeObjectLive(t *testing.T) {
	// APRS101 p.58 worked example (live object).
	f, ok := decodeObject(";LEADER   *092345z4903.50N/07201.75W>088/036")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.kind != "object" || f.name != "LEADER" {
		t.Fatalf("kind=%q name=%q", f.kind, f.name)
	}
	if !f.live {
		t.Fatal("'*' must mean live")
	}
	if !almostEqual(f.lat, 49+3.50/60, 1e-9) || !almostEqual(f.lon, -(72+1.75/60), 1e-9) {
		t.Fatalf("lat/lon = %v,%v", f.lat, f.lon)
	}
	if !f.hasCourse || f.courseDeg != 88 || !f.hasSpeed || f.speedKts != 36 {
		t.Fatalf("course/speed = %v/%v", f.courseDeg, f.speedKts)
	}
}

func TestDecodeObjectKilled(t *testing.T) {
	// Same example as above, "now killed" (APRS101 p.58). The spec renders
	// the kill flag as a highlighted space in its typeset examples; this is
	// a literal " ", not an underscore.
	f, ok := decodeObject(";LEADER    092345z4903.50N/07201.75W>088/036")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.live {
		t.Fatal("a space kill-flag must mean killed")
	}
}

func TestDecodeObjectCompressed(t *testing.T) {
	// APRS101 p.58 compressed-position object example.
	f, ok := decodeObject(";LEADER   *092345z/5L!!<*e7>7P[")
	if !ok {
		t.Fatal("expected a decode")
	}
	if !almostEqual(f.lat, 49.5, 1e-3) || !almostEqual(f.lon, -72.75, 1e-3) {
		t.Fatalf("lat/lon = %v,%v", f.lat, f.lon)
	}
}

func TestDecodeObjectNameIsTrimmed(t *testing.T) {
	f, ok := decodeObject(";AID#2    *092345z4903.50N/07201.75WA")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.name != "AID#2" {
		t.Fatalf("name = %q, want trimmed AID#2", f.name)
	}
}

func TestDecodeObjectRejectsShort(t *testing.T) {
	if _, ok := decodeObject(";TOOSHORT"); ok {
		t.Fatal("expected rejection")
	}
}

func TestDecodeObjectRejectsBadFlag(t *testing.T) {
	if _, ok := decodeObject(";LEADER   X092345z4903.50N/07201.75W>088/036"); ok {
		t.Fatal("only '*' or ' ' are valid live/kill flags")
	}
}

func TestDecodeItemLive(t *testing.T) {
	// APRS101 p.59 worked example.
	f, ok := decodeItem(")AID#2!4903.50N/07201.75WA")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.kind != "item" || f.name != "AID#2" {
		t.Fatalf("kind=%q name=%q", f.kind, f.name)
	}
	if !f.live {
		t.Fatal("'!' must mean live")
	}
	if f.symTable != '/' || f.symCode != 'A' {
		t.Fatalf("symbol = %c%c, want /A (Aid Station)", f.symTable, f.symCode)
	}
}

func TestDecodeItemKilled(t *testing.T) {
	f, ok := decodeItem(")AID#2 4903.50N/07201.75WA")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.live {
		t.Fatal("a space terminator must mean killed")
	}
}

func TestDecodeItemCompressed(t *testing.T) {
	// APRS101 p.59: Mobile Gas Station example.
	f, ok := decodeItem(")MOBIL!\\5L!!<*e79 sT")
	if !ok {
		t.Fatal("expected a decode")
	}
	if !almostEqual(f.lat, 49.5, 1e-3) || !almostEqual(f.lon, -72.75, 1e-3) {
		t.Fatalf("lat/lon = %v,%v", f.lat, f.lon)
	}
	if f.symTable != '\\' || f.symCode != '9' {
		t.Fatalf("symbol = %c%c, want \\9 (Gas Station)", f.symTable, f.symCode)
	}
}

func TestDecodeItemNameLengthBounds(t *testing.T) {
	// Names are 3-9 characters; a 2-character name is invalid.
	if _, ok := decodeItem(")AB!4903.50N/07201.75WA"); ok {
		t.Fatal("a 2-character item name must be rejected")
	}
}
