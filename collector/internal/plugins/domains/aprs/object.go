package aprs

import "strings"

// decodeObject decodes an Object report (data type indicator ";",
// APRS101 chapter 11): a fixed 9-byte name, a live/kill flag, a mandatory
// 7-byte timestamp (its value is not used - see domain.go for why), then a
// position body identical in shape to a plain position report.
func decodeObject(info string) (fix, bool) {
	const nameLen = 9
	const headerLen = 1 + nameLen + 1 + 7 // ';' + name + flag + timestamp
	if len(info) < headerLen {
		return fix{}, false
	}
	name := info[1 : 1+nameLen]
	liveByte := info[1+nameLen]
	var live bool
	switch liveByte {
	case '*':
		live = true
	case ' ':
		live = false
	default:
		return fix{}, false
	}
	f, ok := decodePositionBody(info[headerLen:])
	if !ok {
		return fix{}, false
	}
	f.kind = "object"
	f.name = strings.TrimSpace(name)
	f.live = live
	return f, true
}

// decodeItem decodes an Item report (data type indicator ")",
// APRS101 chapter 11): a variable-length (3-9 byte) name terminated by "!"
// (live) or " " (killed), no timestamp, then a position body.
func decodeItem(info string) (fix, bool) {
	if len(info) < 1 {
		return fix{}, false
	}
	rest := info[1:]
	limit := len(rest)
	if limit > 10 {
		limit = 10 // longest possible name (9) plus its terminator
	}
	term := -1
	for i := 0; i < limit; i++ {
		if rest[i] == '!' || rest[i] == ' ' {
			term = i
			break
		}
	}
	if term < 3 || term > 9 {
		return fix{}, false
	}
	f, ok := decodePositionBody(rest[term+1:])
	if !ok {
		return fix{}, false
	}
	f.kind = "item"
	f.name = rest[:term]
	f.live = rest[term] == '!'
	return f, true
}
