package aprs

import (
	"regexp"
	"strings"
)

// amateurCallPattern is a conservative AX.25 / ITU amateur callsign:
// 1-2 alphanumeric prefix, one digit, 1-4 letters, optional SSID. Used only
// to decide whether an Object/Item name is the same station as a position
// report (HB9SVT-5 as both a station and an object) rather than a distinct
// named object (LEADER, MEETING).
var amateurCallPattern = regexp.MustCompile(`^[A-Z0-9]{1,2}[0-9][A-Z]{1,4}(-[0-9A-Z]{1,2})?$`)

func normalizeCallsign(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

func looksLikeStationCall(s string) bool {
	return amateurCallPattern.MatchString(normalizeCallsign(s))
}

// entityIdentity derives the stable per-entity ID and display title. A
// plain position report (or Mic-E, which carries the real callsign in the
// AX.25 source field even though its destination field is repurposed for
// position data) is identified by its transmitting callsign. An Object or
// Item whose name is itself an amateur callsign is the same station
// (trackers and igates routinely beacon both shapes); any other name stays
// an independent object/item so replacing or killing it remains a normal
// part of the protocol (APRS101 chapter 11).
func entityIdentity(packet tnc2Packet, f fix) (entityID, title string) {
	switch f.kind {
	case "object", "item":
		name := normalizeCallsign(f.name)
		if looksLikeStationCall(f.name) {
			return "aprs:station:" + name, name
		}
		return "aprs:" + f.kind + ":" + name, strings.TrimSpace(f.name)
	default:
		call := normalizeCallsign(packet.source)
		return "aprs:station:" + call, call
	}
}

// unwrapThirdParty peels at most two third-party hops (`}` + inner TNC2).
// The inner source is the station that actually reported the position;
// treating the igate wrapper as its own marker is what stacked duplicates.
func unwrapThirdParty(packet tnc2Packet) (tnc2Packet, bool) {
	current := packet
	for hop := 0; hop < 2; hop++ {
		if current.info == "" || current.info[0] != '}' {
			return current, true
		}
		inner, ok := parseTNC2(current.info[1:])
		if !ok {
			return tnc2Packet{}, false
		}
		current = inner
	}
	if current.info != "" && current.info[0] == '}' {
		return tnc2Packet{}, false
	}
	return current, true
}
