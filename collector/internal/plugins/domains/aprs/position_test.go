package aprs

import (
	"math"
	"testing"
)

func almostEqual(a, b, tolerance float64) bool { return math.Abs(a-b) <= tolerance }

func TestDecodeUncompressedBasic(t *testing.T) {
	// APRS101 chapter 8 example: "4903.50N/07201.75W-" is 49 deg 3.50 min
	// N, 072 deg 1.75 min W, symbol table "/", symbol code "-" (house).
	f, ok := decodeUncompressed("4903.50N/07201.75W-")
	if !ok {
		t.Fatal("expected a decode")
	}
	if !almostEqual(f.lat, 49+3.50/60, 1e-9) {
		t.Fatalf("lat = %v", f.lat)
	}
	if !almostEqual(f.lon, -(72 + 1.75/60), 1e-9) {
		t.Fatalf("lon = %v", f.lon)
	}
	if f.symTable != '/' || f.symCode != '-' {
		t.Fatalf("symbol = %c%c", f.symTable, f.symCode)
	}
}

func TestDecodeUncompressedSouthEastHemispheres(t *testing.T) {
	f, ok := decodeUncompressed("4903.50S/07201.75E-")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.lat >= 0 {
		t.Fatalf("south latitude must be negative, got %v", f.lat)
	}
	if f.lon <= 0 {
		t.Fatalf("east longitude must be positive, got %v", f.lon)
	}
}

func TestDecodeUncompressedPositionAmbiguity(t *testing.T) {
	// APRS101 p.24: replacing trailing digits with spaces reduces
	// precision; "49  .  N" (nearest degree) must not fail to parse.
	f, ok := decodeUncompressed("49  .  N/072  .  W-")
	if !ok {
		t.Fatal("ambiguous position must still decode")
	}
	if f.lat != 49 || f.lon != -72 {
		t.Fatalf("expected 49,-72 with ambiguous digits treated as 0, got %v,%v", f.lat, f.lon)
	}
}

func TestDecodeUncompressedRejectsGarbage(t *testing.T) {
	if _, ok := decodeUncompressed("not a position at all"); ok {
		t.Fatal("garbage must not parse as a position")
	}
	if _, ok := decodeUncompressed("4903.50X/07201.75W-"); ok {
		t.Fatal("invalid hemisphere letter must be rejected")
	}
}

func TestParseCourseSpeedExtension(t *testing.T) {
	course, speed, remainder, ok := parseCourseSpeedExtension("088/036/A=001234", '/', '-')
	if !ok {
		t.Fatal("expected a recognised extension")
	}
	if course == nil || *course != 88 {
		t.Fatalf("course = %v", course)
	}
	if speed == nil || *speed != 36 {
		t.Fatalf("speed = %v", speed)
	}
	if remainder != "/A=001234" {
		t.Fatalf("remainder = %q", remainder)
	}
}

func TestParseCourseSpeedExtensionUnknownSentinel(t *testing.T) {
	for _, head := range []string{"000/000", ".../...", "   /   "} {
		course, speed, remainder, ok := parseCourseSpeedExtension(head+"comment", '/', '>')
		if !ok {
			t.Fatalf("shape %q should still be a recognised (if unknown) extension", head)
		}
		if course != nil || speed != nil {
			t.Fatalf("sentinel %q must not produce a course/speed", head)
		}
		if remainder != "comment" {
			t.Fatalf("remainder = %q", remainder)
		}
	}
}

func TestParseCourseSpeedExtensionZeroSpeedIsReal(t *testing.T) {
	// Unlike course, "000" speed is a genuine, valid reading (stationary):
	// speed's documented range is 0-799, course's is 001-360.
	course, speed, _, ok := parseCourseSpeedExtension("090/000hello", '/', '>')
	if !ok {
		t.Fatal("expected a recognised extension")
	}
	if course == nil || *course != 90 {
		t.Fatalf("course = %v, want 90", course)
	}
	if speed == nil || *speed != 0 {
		t.Fatalf("speed = %v, want 0 (stationary, not unknown)", speed)
	}
}

func TestParseCourseSpeedExtensionSkipsPHGRNGDFS(t *testing.T) {
	for _, prefix := range []string{"PHG5132", "RNG0050", "DFS2360"} {
		course, speed, remainder, ok := parseCourseSpeedExtension(prefix+"hello", '/', '#')
		if !ok {
			t.Fatalf("%s must be recognised and skipped", prefix)
		}
		if course != nil || speed != nil {
			t.Fatalf("%s must not be misread as course/speed", prefix)
		}
		if remainder != "hello" {
			t.Fatalf("remainder after %s = %q", prefix, remainder)
		}
	}
}

func TestParseCourseSpeedExtensionAreaObjectNotMisread(t *testing.T) {
	// APRS101 p.60/61: "710/310" is a Tyy/Cxx Area Object descriptor (a
	// high intensity cyan filled ellipse, yy=10, xx=10), not course 710.
	_, _, remainder, ok := parseCourseSpeedExtension("710/310", '\\', 'l')
	if ok {
		t.Fatal("Area Object descriptor must not be consumed as CSE/SPD")
	}
	if remainder != "710/310" {
		t.Fatalf("remainder must be untouched, got %q", remainder)
	}
}

func TestParseCourseSpeedExtensionRejectsOutOfRangeCourse(t *testing.T) {
	// Safety net for the case above even without the Area Object symbol:
	// a "course" over 360 is impossible and must not be trusted.
	course, speed, _, ok := parseCourseSpeedExtension("710/310", '/', '>')
	if !ok {
		t.Fatal("shape still matches xxx/yyy, so it is consumed")
	}
	if course != nil || speed != nil {
		t.Fatal("an out-of-range course must not be surfaced")
	}
}

func TestExtractCommentAltitude(t *testing.T) {
	alt, cleaned := extractCommentAltitude("Test /A=001234 comment")
	if alt == nil || !almostEqual(*alt, 1234*feetToMeters, 1e-6) {
		t.Fatalf("altitude = %v", alt)
	}
	if cleaned != "Test  comment" {
		t.Fatalf("cleaned comment = %q", cleaned)
	}
}

func TestExtractCommentAltitudeAbsent(t *testing.T) {
	alt, cleaned := extractCommentAltitude("just a comment")
	if alt != nil {
		t.Fatal("no altitude token present")
	}
	if cleaned != "just a comment" {
		t.Fatalf("comment must be unchanged, got %q", cleaned)
	}
}

// TestDecodeCompressedSpecExample reproduces APRS101 chapter 9's own worked
// example end to end (pages 38-40): latitude 49 deg 30' 00" N, longitude 72
// deg 45' 00" W, course 88, speed 36.2 kn, symbol "car", fix from an RMC
// sentence. The expected values below were hand-verified against the
// spec's stated intermediate arithmetic (see the design notes), not just
// copied from its prose - in particular 380926 and 190463 divide the
// encoded integers to reproduce 49.5 and -72.75 almost exactly (residual
// error under 4e-6 degrees, inherent to the format's own quantisation).
func TestDecodeCompressedSpecExample(t *testing.T) {
	f, ok := decodeCompressed("/5L!!<*e7>7P[")
	if !ok {
		t.Fatal("expected a decode")
	}
	if !almostEqual(f.lat, 49.5, 1e-3) {
		t.Fatalf("lat = %v, want ~49.5", f.lat)
	}
	if !almostEqual(f.lon, -72.75, 1e-3) {
		t.Fatalf("lon = %v, want ~-72.75", f.lon)
	}
	if f.symTable != '/' || f.symCode != '>' {
		t.Fatalf("symbol = %c%c, want /> (car)", f.symTable, f.symCode)
	}
	if !f.hasCourse || !almostEqual(f.courseDeg, 88, 1e-9) {
		t.Fatalf("course = %v", f.courseDeg)
	}
	if !f.hasSpeed || !almostEqual(f.speedKts, 36.2, 0.05) {
		t.Fatalf("speed = %v, want ~36.2", f.speedKts)
	}
	if f.hasAlt {
		t.Fatal("this example is course/speed (RMC source), not altitude (GGA)")
	}
}

// TestDecodeCompressedAltitude reuses the same lat/lon field but a cs/T
// combination whose T byte marks a GGA source, which APRS101 p.40 states
// decodes cs="S]" to 10004 feet (the doc's own worked sub-example).
func TestDecodeCompressedAltitude(t *testing.T) {
	f, ok := decodeCompressed("/5L!!<*e7>S]1")
	if !ok {
		t.Fatal("expected a decode")
	}
	if !f.hasAlt {
		t.Fatal("expected altitude from a GGA-sourced compressed position")
	}
	wantMeters := 10004.0 * feetToMeters
	if !almostEqual(f.altMeters, wantMeters, 2.0) {
		t.Fatalf("altitude = %v m, want ~%v m", f.altMeters, wantMeters)
	}
	if f.hasCourse || f.hasSpeed {
		t.Fatal("a GGA-sourced fix must not also report course/speed")
	}
}

func TestDecodeCompressedNoCourseSpeedData(t *testing.T) {
	f, ok := decodeCompressed("/5L!!<*e7>   1")
	if !ok {
		t.Fatal("expected a decode")
	}
	if f.hasCourse || f.hasSpeed || f.hasAlt {
		t.Fatal("a space c-byte means no course/speed/range/altitude data")
	}
}

func TestDecodeCompressedRejectsTooShort(t *testing.T) {
	if _, ok := decodeCompressed("/5L!!<*e7"); ok {
		t.Fatal("a truncated compressed field must be rejected")
	}
}

func TestBase91Decode4(t *testing.T) {
	value, ok := base91Decode4("5L!!")
	if !ok || value != 15427503 {
		t.Fatalf("base91Decode4(5L!!) = %v, %v, want 15427503", value, ok)
	}
}

func TestDecodePositionBodyDispatchesOnFirstByte(t *testing.T) {
	if _, ok := decodePositionBody(""); ok {
		t.Fatal("empty body must be rejected")
	}
	uncompressed, ok := decodePositionBody("4903.50N/07201.75W-")
	if !ok || uncompressed.symCode != '-' {
		t.Fatal("digit-leading body must use the uncompressed decoder")
	}
	compressed, ok := decodePositionBody("/5L!!<*e7>7P[")
	if !ok || compressed.symCode != '>' {
		t.Fatal("non-digit-leading body must use the compressed decoder")
	}
}
