package aprs

import (
	"math"
	"strconv"
	"strings"
)

// fix is the geometry and presentation data decoded from one APRS packet,
// regardless of which packet type produced it (plain position report,
// object, item or Mic-E). Course/speed/altitude are optional because most
// packets do not carry all of them.
type fix struct {
	lon, lat float64

	symTable byte
	symCode  byte

	hasCourse bool
	courseDeg float64
	hasSpeed  bool
	speedKts  float64
	hasAlt    bool
	altMeters float64

	comment string

	// name and kind are set by the object/item decoders; a plain position
	// report leaves name empty and kind "station" so the caller falls back
	// to the packet's source callsign as the identity.
	kind string // "station" | "object" | "item"
	name string
	live bool // objects/items only: false means a kill report

	// micE carries the decoded Mic-E status-message classification, when
	// the fix came from a Mic-E packet.
	micE *micEMessage
}

// decodePositionBody decodes the position portion shared by plain position
// reports, objects and items: everything from right after the data-type
// indicator (and, for timestamped/object reports, right after the
// timestamp) onward. The APRS spec disambiguates compressed from
// uncompressed purely by the first byte: a digit means uncompressed,
// anything else means the byte is a compressed-format symbol table ID.
func decodePositionBody(body string) (fix, bool) {
	if body == "" {
		return fix{}, false
	}
	if body[0] >= '0' && body[0] <= '9' {
		return decodeUncompressed(body)
	}
	return decodeCompressed(body)
}

// decodeUncompressed parses the classic "DDMM.hhN/DDDMM.hhW-" layout
// (APRS101 chapter 6/8): 8-byte latitude, 1-byte symbol table ID, 9-byte
// longitude, 1-byte symbol code, then an optional data extension and free
// text comment.
func decodeUncompressed(body string) (fix, bool) {
	const fixedLen = 19 // 8 (lat) + 1 (table) + 9 (lon) + 1 (code)
	if len(body) < fixedLen {
		return fix{}, false
	}
	latField := body[0:7]
	hemiNS := body[7]
	symTable := body[8]
	lonField := body[9:17]
	hemiEW := body[17]
	symCode := body[18]
	rest := body[fixedLen:]

	lat, ok := decodeDegMin(latField, 2)
	if !ok {
		return fix{}, false
	}
	switch hemiNS {
	case 'S':
		lat = -lat
	case 'N':
	default:
		return fix{}, false
	}
	lon, ok := decodeDegMin(lonField, 3)
	if !ok {
		return fix{}, false
	}
	switch hemiEW {
	case 'W':
		lon = -lon
	case 'E':
	default:
		return fix{}, false
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return fix{}, false
	}

	f := fix{lon: lon, lat: lat, symTable: symTable, symCode: symCode}
	comment := rest
	if course, speedKts, extRest, ok := parseCourseSpeedExtension(rest, symTable, symCode); ok {
		if course != nil {
			f.hasCourse, f.courseDeg = true, *course
		}
		if speedKts != nil {
			f.hasSpeed, f.speedKts = true, *speedKts
		}
		comment = extRest
	}
	if altMeters, cleaned := extractCommentAltitude(comment); altMeters != nil {
		f.hasAlt, f.altMeters = true, *altMeters
		comment = cleaned
	}
	f.comment = strings.TrimSpace(comment)
	return f, true
}

// decodeDegMin parses a fixed-width "DDMM.hh" (degDigits=2, latitude) or
// "DDDMM.hh" (degDigits=3, longitude) field into decimal degrees. A space in
// place of a digit is the documented position-ambiguity marker; it is
// treated as 0, which is the safe, widely used simplification (the true
// value only ever differs by less than the stated ambiguity).
func decodeDegMin(field string, degDigits int) (float64, bool) {
	if len(field) != degDigits+5 {
		return 0, false
	}
	if field[degDigits+2] != '.' {
		return 0, false
	}
	deg, ok := parseAmbiguousInt(field[0:degDigits])
	if !ok {
		return 0, false
	}
	min, ok := parseAmbiguousInt(field[degDigits : degDigits+2])
	if !ok {
		return 0, false
	}
	hundredths, ok := parseAmbiguousInt(field[degDigits+3 : degDigits+5])
	if !ok {
		return 0, false
	}
	return float64(deg) + (float64(min)+float64(hundredths)/100.0)/60.0, true
}

func parseAmbiguousInt(s string) (int, bool) {
	s = strings.ReplaceAll(s, " ", "0")
	value, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return value, true
}

// parseCourseSpeedExtension recognises the 7-byte CSE/SPD data extension
// ("ccc/sss", degrees clockwise from north / knots) that may immediately
// follow the symbol code. PHG, RNG and DFS extensions have the same slot but
// a distinct literal prefix and are not decoded further, only skipped so
// they do not leak into the comment. The Area Object extension (Tyy/Cxx)
// shares the CSE/SPD's "xxx/yyy" shape, so it is recognised first via the
// Area Object symbol (alternate table, lowercase "l") and left alone.
// ok reports whether a 7-byte extension was recognised and consumed at all
// (course/speed themselves may still be nil, e.g. "000/000" = unknown).
func parseCourseSpeedExtension(rest string, symTable, symCode byte) (course, speedKts *float64, remainder string, ok bool) {
	if len(rest) < 7 {
		return nil, nil, rest, false
	}
	head := rest[0:7]
	if symTable == '\\' && symCode == 'l' {
		// Area Object descriptor: a shape/color extension, not a position.
		return nil, nil, rest, false
	}
	switch {
	case strings.HasPrefix(head, "PHG"), strings.HasPrefix(head, "RNG"), strings.HasPrefix(head, "DFS"):
		return nil, nil, rest[7:], true
	}
	if head[3] != '/' {
		return nil, nil, rest, false
	}
	courseVal, courseKnown, courseOk := parseTriple(head[0:3])
	speedVal, speedKnown, speedOk := parseTriple(head[4:7])
	if !courseOk || !speedOk {
		return nil, nil, rest, false
	}
	remainder = rest[7:]
	if courseVal == 0 {
		// Course is documented as 001-360; "000" (unlike "000" speed, which
		// legitimately means stationary) is itself one of the "unknown or
		// not relevant" sentinels, alongside "..." and blank (APRS101 ch.7).
		courseKnown = false
	}
	if courseKnown && speedKnown && courseVal >= 1 && courseVal <= 360 && speedVal >= 0 {
		c := float64(courseVal)
		s := float64(speedVal)
		return &c, &s, remainder, true
	}
	return nil, nil, remainder, true
}

// parseTriple decodes one 3-character CSE or SPD field. The unknown
// sentinels documented in APRS101 (chapter 7) are "..." and three spaces, in
// addition to "000".
func parseTriple(s string) (value int, known, ok bool) {
	if len(s) != 3 {
		return 0, false, false
	}
	if s == "..." || s == "   " {
		return 0, false, true
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false, false
	}
	return v, true, true
}

// extractCommentAltitude finds the "/A=aaaaaa" (feet, 6 digits) altitude
// token documented to be able to appear anywhere in the comment, and
// returns the altitude plus the comment with that token removed so the
// tooltip does not show the same altitude twice.
func extractCommentAltitude(comment string) (*float64, string) {
	idx := strings.Index(comment, "/A=")
	if idx < 0 || idx+9 > len(comment) {
		return nil, comment
	}
	digits := comment[idx+3 : idx+9]
	for _, c := range digits {
		if c < '0' || c > '9' {
			return nil, comment
		}
	}
	feet, err := strconv.Atoi(digits)
	if err != nil {
		return nil, comment
	}
	meters := float64(feet) * feetToMeters
	cleaned := strings.TrimSpace(comment[:idx] + comment[idx+9:])
	return &meters, cleaned
}

const feetToMeters = 0.3048
const knotsToMps = 0.514444

// base91Decode4 decodes a 4-character base-91 field (APRS101 chapter 9):
// each byte's value is its ASCII code minus 33, combined with Horner's
// method (base 91), valid range "!".."{" (33-123, i.e. digit values 0-90).
func base91Decode4(field string) (int64, bool) {
	if len(field) != 4 {
		return 0, false
	}
	var value int64
	for i := 0; i < 4; i++ {
		c := field[i]
		if c < 33 || c > 123 {
			return 0, false
		}
		value = value*91 + int64(c-33)
	}
	return value, true
}

// decodeCompressed parses the 13-byte compressed position field (APRS101
// chapter 9): symbol table ID, 4-byte compressed latitude, 4-byte
// compressed longitude, symbol code, then a 3-byte course/speed, radio
// range, or (bytes 4-3 of the compression-type byte are the origin was a
// GGA sentence) altitude field.
func decodeCompressed(body string) (fix, bool) {
	const fixedLen = 13
	if len(body) < fixedLen {
		return fix{}, false
	}
	symTable := body[0]
	latVal, ok := base91Decode4(body[1:5])
	if !ok {
		return fix{}, false
	}
	lonVal, ok := base91Decode4(body[5:9])
	if !ok {
		return fix{}, false
	}
	symCode := body[9]
	c0, c1, tByte := body[10], body[11], body[12]
	rest := body[fixedLen:]

	lat := 90.0 - float64(latVal)/380926.0
	lon := -180.0 + float64(lonVal)/190463.0
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return fix{}, false
	}

	f := fix{lon: lon, lat: lat, symTable: symTable, symCode: symCode}

	// The compression-type byte's NMEA-source bits (4:3) take priority over
	// the c0-based dispatch below: the worked example in APRS101 chapter 9
	// decodes a c0 value that also happens to fall in the course/speed
	// range as altitude, specifically because the T byte says the fix came
	// from a GGA sentence (which carries altitude, not course/speed).
	if tVal := int(tByte) - 33; tVal >= 0 {
		nmeaSource := (tVal >> 3) & 0b11
		const gga = 0b10
		if nmeaSource == gga && c0 != ' ' {
			cs := (int(c0)-33)*91 + (int(c1) - 33)
			if cs >= 0 {
				feet := math.Pow(1.002, float64(cs))
				meters := feet * feetToMeters
				f.hasAlt, f.altMeters = true, meters
			}
		}
	}
	if !f.hasAlt {
		switch {
		case c0 == ' ':
			// No course/speed/range data; csT is ignored entirely.
		case c0 == '{':
			// Pre-calculated radio range: consumed, not surfaced.
		default:
			if cVal := int(c0) - 33; cVal >= 0 && cVal <= 89 {
				sVal := int(c1) - 33
				if sVal >= 0 {
					f.hasCourse, f.courseDeg = true, float64(cVal)*4
					f.hasSpeed, f.speedKts = true, math.Pow(1.08, float64(sVal))-1
				}
			}
		}
	}

	comment := rest
	if altMeters, cleaned := extractCommentAltitude(comment); altMeters != nil && !f.hasAlt {
		f.hasAlt, f.altMeters = true, *altMeters
		comment = cleaned
	}
	f.comment = strings.TrimSpace(comment)
	return f, true
}
