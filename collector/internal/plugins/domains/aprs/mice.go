package aprs

import "strings"

// Mic-E packets (APRS101 chapter 10) pack latitude, message type and
// longitude sign/offset into the AX.25 destination address, and longitude,
// symbol and status into the information field. This decoder covers
// position, symbol and message-status decoding. Course/speed decoding is
// deliberately NOT implemented: APRS101's own worked example (page 53)
// requires an undocumented "subtract 80 if >= 80" normalisation of the
// SP+28 byte that contradicts the literal 3-step algorithm given a few
// pages earlier, and the DC+28 dual-encoding table cannot be reproduced
// from that same literal algorithm either (see position_test.go for the
// arithmetic that exposes the contradiction). Shipping a plausible-looking
// but unverified course/speed decoder was judged worse than clearly
// omitting it.

// micEHalf is one decoded byte of the Mic-E destination address field.
type micEHalf struct {
	digit    int  // 0-9, or -1 for the position-ambiguity space marker
	extended bool // true = "high half" (P-Z/Z): North / +100 offset / West
	custom   bool // true = "custom" low half (A-K): only valid in bytes 1-3
}

// micEChar decodes one byte of the 6-byte Mic-E destination address core
// (the SSID, if any, must already be stripped). See APRS101 page 44.
func micEChar(c byte) (micEHalf, bool) {
	switch {
	case c >= '0' && c <= '9':
		return micEHalf{digit: int(c - '0')}, true
	case c == 'L':
		return micEHalf{digit: -1}, true
	case c >= 'A' && c <= 'J':
		return micEHalf{digit: int(c - 'A'), custom: true}, true
	case c == 'K':
		return micEHalf{digit: -1, custom: true}, true
	case c >= 'P' && c <= 'Y':
		return micEHalf{digit: int(c - 'P'), extended: true}, true
	case c == 'Z':
		return micEHalf{digit: -1, extended: true}, true
	default:
		return micEHalf{}, false
	}
}

func digitOrZero(h micEHalf) int {
	if h.digit < 0 {
		return 0
	}
	return h.digit
}

type micEMessage struct {
	kind  string // "std" | "custom" | "emergency" | "unknown"
	label string
}

// Indexed by the 3-bit A/B/C message identifier (APRS101 page 45); index 0
// is the reserved all-zero pattern, which means Emergency regardless of
// source (custom vs standard).
var stdMicEMessages = [8]string{"Emergency", "M6: Priority", "M5: Special", "M4: Committed", "M3: Returning", "M2: In Service", "M1: En Route", "M0: Off Duty"}
var customMicEMessages = [8]string{"Emergency", "C6: Custom-6", "C5: Custom-5", "C4: Custom-4", "C3: Custom-3", "C2: Custom-2", "C1: Custom-1", "C0: Custom-0"}

// decodeMicEMessage classifies the 3-bit A/B/C message identifier spread
// across the first 3 destination-address bytes. A mixture of "custom" and
// "standard" source bytes is documented as producing an "unknown" message.
func decodeMicEMessage(bits [3]micEHalf) micEMessage {
	hasCustom, hasStd, index := false, false, 0
	for _, half := range bits {
		bit := 0
		if half.custom {
			hasCustom, bit = true, 1
		}
		if half.extended {
			hasStd, bit = true, 1
		}
		index = (index << 1) | bit
	}
	switch {
	case hasCustom && hasStd:
		return micEMessage{kind: "unknown"}
	case index == 0:
		return micEMessage{kind: "emergency", label: stdMicEMessages[0]}
	case hasCustom:
		return micEMessage{kind: "custom", label: customMicEMessages[index]}
	default:
		return micEMessage{kind: "std", label: stdMicEMessages[index]}
	}
}

// stripSSID removes a trailing "-NN" SSID from a TNC2 callsign-shaped field.
func stripSSID(field string) string {
	if i := strings.IndexByte(field, '-'); i >= 0 {
		return field[:i]
	}
	return field
}

// decodeMicE decodes a Mic-E packet (data type indicator "`" or "'") from
// its full TNC2 destination field and information field. See the package
// comment for why course/speed is intentionally left undecoded.
func decodeMicE(dest, info string) (fix, bool) {
	destCore := stripSSID(dest)
	if len(destCore) != 6 {
		return fix{}, false
	}
	var halves [6]micEHalf
	for i := 0; i < 6; i++ {
		h, ok := micEChar(destCore[i])
		if !ok {
			return fix{}, false
		}
		halves[i] = h
	}
	// Bytes 4-6 (N/S, longitude offset, W/E) never use the "custom" range.
	for i := 3; i < 6; i++ {
		if halves[i].custom {
			return fix{}, false
		}
	}

	latDeg := digitOrZero(halves[0])*10 + digitOrZero(halves[1])
	latMin := digitOrZero(halves[2])*10 + digitOrZero(halves[3])
	latHun := digitOrZero(halves[4])*10 + digitOrZero(halves[5])
	lat := float64(latDeg) + (float64(latMin)+float64(latHun)/100.0)/60.0
	if !halves[3].extended { // byte 4 clear -> South
		lat = -lat
	}
	longOffset := 0
	if halves[4].extended { // byte 5 set -> longitude degrees + 100
		longOffset = 100
	}
	west := halves[5].extended // byte 6 set -> West

	// Information field: the data type char has already been consumed by
	// the caller, so the 8 bytes here are longitude (d+28,m+28,h+28),
	// speed/course (SP+28,DC+28,SE+28 - not decoded, see package comment),
	// symbol code, symbol table ID; anything after that is telemetry or
	// free-text status.
	if len(info) < 8 {
		return fix{}, false
	}
	dRaw := int(info[0]) - 28
	mRaw := int(info[1]) - 28
	hRaw := int(info[2]) - 28
	symCode := info[6]
	symTable := info[7]
	if dRaw < 0 || mRaw < 0 || hRaw < 0 || hRaw > 99 {
		return fix{}, false
	}
	d := dRaw
	if longOffset == 100 {
		d += 100
	}
	switch {
	case d >= 180 && d <= 189:
		d -= 80
	case d >= 190 && d <= 199:
		d -= 190
	}
	if d < 0 || d > 179 {
		return fix{}, false
	}
	if mRaw >= 60 {
		mRaw -= 60
	}
	if mRaw > 59 {
		return fix{}, false
	}
	lon := float64(d) + (float64(mRaw)+float64(hRaw)/100.0)/60.0
	if west {
		lon = -lon
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return fix{}, false
	}

	comment := ""
	if len(info) > 8 {
		status := info[8:]
		// Telemetry data (hex or binary channel values) is not a comment;
		// showing it as one would produce tooltip noise, not text.
		if len(status) == 0 || (status[0] != '`' && status[0] != '\'' && status[0] != 0x1d) {
			comment = status
		}
	}
	message := decodeMicEMessage([3]micEHalf{halves[0], halves[1], halves[2]})
	f := fix{lon: lon, lat: lat, symTable: symTable, symCode: symCode, micE: &message}
	if altMeters, cleaned := extractCommentAltitude(comment); altMeters != nil {
		f.hasAlt, f.altMeters = true, *altMeters
		comment = cleaned
	}
	f.comment = strings.TrimSpace(comment)
	return f, true
}
