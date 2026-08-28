package aprs

import "strings"

// tnc2Packet is one parsed "SRC>DEST,PATH1,PATH2,...:INFO" line - the
// text-frame representation APRS-IS uses for what was originally an AX.25
// UI-frame. Splitting this shape is IS/AX.25 framing, not ham vocabulary by
// itself; the domain-specific meaning starts once info's first byte is
// interpreted as an APRS data type indicator.
type tnc2Packet struct {
	source string
	dest   string
	path   []string
	info   string
}

// parseTNC2 splits a raw line into its framing parts. Only the first ">" and
// the first ":" are structurally significant - the info field is free to
// contain either character, so no further split is attempted.
func parseTNC2(line string) (tnc2Packet, bool) {
	gt := strings.IndexByte(line, '>')
	if gt <= 0 {
		return tnc2Packet{}, false
	}
	rest := line[gt+1:]
	colon := strings.IndexByte(rest, ':')
	if colon <= 0 {
		return tnc2Packet{}, false
	}
	header := rest[:colon]
	info := rest[colon+1:]
	if info == "" {
		return tnc2Packet{}, false
	}
	parts := strings.Split(header, ",")
	if parts[0] == "" {
		return tnc2Packet{}, false
	}
	return tnc2Packet{source: line[:gt], dest: parts[0], path: parts[1:], info: info}, true
}

// lastPathHop returns the final via-path element (typically the IGate or
// digipeater that delivered the packet to APRS-IS), if any.
func (p tnc2Packet) lastPathHop() string {
	if len(p.path) == 0 {
		return ""
	}
	hop := p.path[len(p.path)-1]
	// Strip the AX.25 "has-been-repeated" marker some igates leave on.
	return strings.TrimSuffix(hop, "*")
}
