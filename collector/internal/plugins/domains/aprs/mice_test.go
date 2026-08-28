package aprs

import "testing"

// TestDecodeMicESpecDestinationExample reproduces APRS101 page 44's own
// destination-address worked example: latitude 33 deg 25.64' N, message
// bits 1/0/0 (Standard), longitude offset +0, West - which the spec states
// encodes to destination bytes "S32U6T". Reproducing that byte string here
// (rather than trusting a hand-copied number) is the cross-check: it only
// matches if micEChar's per-byte table (independently built from the
// "Destination Address Field Encoding" grid, not this worked example) is
// consistent with the spec's own worked answer, and the page-46 message
// table separately confirms bits 1/0/0 = "M3: Returning".
func TestDecodeMicESpecDestinationExample(t *testing.T) {
	// Longitude/symbol chosen independently from the clean encoding tables
	// (not from the harder-to-OCR inline worked prose) - see design notes:
	// d+28='(' -> 12 deg (+0 offset), m+28='5' -> 25 min, h+28='N' -> 50
	// hundredths, giving 12 deg 25.50 min. SP/DC/SE are irrelevant filler
	// since course/speed is intentionally not decoded.
	f, ok := decodeMicE("S32U6T-9", "(5NXXXj/Hello")
	if !ok {
		t.Fatal("expected a decode")
	}
	wantLat := 33 + (25+64.0/100)/60
	if !almostEqual(f.lat, wantLat, 1e-9) {
		t.Fatalf("lat = %v, want %v", f.lat, wantLat)
	}
	if f.lat <= 0 {
		t.Fatal("byte 4 'U' is the high half -> North, must be positive")
	}
	wantLon := -(12 + (25+50.0/100)/60)
	if !almostEqual(f.lon, wantLon, 1e-9) {
		t.Fatalf("lon = %v, want %v", f.lon, wantLon)
	}
	if f.symTable != '/' || f.symCode != 'j' {
		t.Fatalf("symbol = %c%c, want /j (jeep)", f.symTable, f.symCode)
	}
	if f.comment != "Hello" {
		t.Fatalf("comment = %q", f.comment)
	}
	if f.micE == nil || f.micE.kind != "std" || f.micE.label != "M3: Returning" {
		t.Fatalf("micE = %+v, want std M3: Returning", f.micE)
	}
}

func TestDecodeMicEEmergency(t *testing.T) {
	// All three message-identifier bits 0 (destination bytes drawn from the
	// "0-9" low-half, non-custom range) means Emergency (APRS101 p.45).
	f, ok := decodeMicE("234567", "(5NXXXj/")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.micE == nil || f.micE.kind != "emergency" {
		t.Fatalf("micE = %+v, want emergency", f.micE)
	}
}

func TestDecodeMicEMixedCustomStandardIsUnknown(t *testing.T) {
	// Byte 1 from the custom (A-K) range, byte 2 from the standard (P-Z)
	// range: APRS101 p.45 says a mixture makes the message type "unknown".
	f, ok := decodeMicE("A3U567", "(5NXXXj/")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.micE == nil || f.micE.kind != "unknown" {
		t.Fatalf("micE = %+v, want unknown", f.micE)
	}
}

func TestDecodeMicERejectsCustomInBytes4to6(t *testing.T) {
	// APRS101 p.44: "the ASCII characters A-K are not used in address
	// bytes 4-6".
	if _, ok := decodeMicE("S32AAA", "(5NXXXj/"); ok {
		t.Fatal("custom-range characters in bytes 4-6 must be rejected")
	}
}

func TestDecodeMicERejectsShortInfo(t *testing.T) {
	if _, ok := decodeMicE("S32U6T", "(5N"); ok {
		t.Fatal("an information field shorter than 9 bytes must be rejected")
	}
}

func TestDecodeMicERejectsWrongDestinationLength(t *testing.T) {
	if _, ok := decodeMicE("S32U6", "(5NXXXj/"); ok {
		t.Fatal("a 5-byte destination core must be rejected")
	}
}

func TestDecodeMicETelemetryNotShownAsComment(t *testing.T) {
	f, ok := decodeMicE("S32U6T", "(5NXXXj/'7200007100")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.comment != "" {
		t.Fatalf("telemetry payload must not leak into the comment, got %q", f.comment)
	}
}

func TestMicECharBoundaries(t *testing.T) {
	cases := []struct {
		c        byte
		digit    int
		extended bool
		custom   bool
	}{
		{'0', 0, false, false},
		{'9', 9, false, false},
		{'L', -1, false, false},
		{'A', 0, false, true},
		{'J', 9, false, true},
		{'K', -1, false, true},
		{'P', 0, true, false},
		{'Y', 9, true, false},
		{'Z', -1, true, false},
	}
	for _, c := range cases {
		half, ok := micEChar(c.c)
		if !ok {
			t.Fatalf("%c: expected a valid decode", c.c)
		}
		if half.digit != c.digit || half.extended != c.extended || half.custom != c.custom {
			t.Fatalf("%c: got %+v, want digit=%d extended=%v custom=%v", c.c, half, c.digit, c.extended, c.custom)
		}
	}
	if _, ok := micEChar('!'); ok {
		t.Fatal("'!' is not a valid Mic-E destination character")
	}
}
