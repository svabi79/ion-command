package aprs

// symbolInfo gives a short human label and a coarse renderer-facing category
// for one APRS symbol. This is deliberately a small, curated subset, not the
// full ~200-entry table from APRS101 chapter 20 / appendix 2: several
// entries below are taken directly from worked examples in the protocol
// spec itself (noted per entry); the rest are well-established, ubiquitous
// symbols. Anything not listed falls back to a generic label - the symbol
// table/code are never invented, only their human name is approximate.
type symbolInfo struct {
	label string
	icon  string // generic category for the renderer's visual.icon
}

// primarySymbols covers the primary symbol table (table ID "/").
var primarySymbols = map[byte]symbolInfo{
	'-': {"House", "house"},     // APRS101 p.24 worked example
	'A': {"Aid Station", "poi"}, // APRS101 p.59 Item example
	'>': {"Car", "vehicle"},
	'<': {"Motorcycle", "vehicle"},
	'#': {"Digipeater", "digipeater"},
	'&': {"Gateway", "gateway"},
	'_': {"Weather Station", "weather"}, // APRS101 ch.12: WX reports use "_"
	'O': {"Balloon", "balloon"},
	'j': {"Jeep", "vehicle"}, // APRS101 p.53 Mic-E worked example
	'k': {"Truck", "vehicle"},
	'R': {"Recreational Vehicle", "vehicle"},
	'b': {"Bicycle", "vehicle"},
	'[': {"Person", "person"},
	'Y': {"Yacht", "boat"},
	's': {"Ship", "boat"},
}

// alternateSymbols covers the alternate symbol table (table ID "\") and,
// for lookup purposes, any overlaid alternate-table character.
var alternateSymbols = map[byte]symbolInfo{
	'd': {"DX Spot", "station"},  // APRS101 p.59 Item example
	'9': {"Gas Station", "poi"},  // APRS101 p.59 Item example
	'm': {"Signpost", "poi"},     // APRS101 p.61
	'l': {"Area Object", "area"}, // APRS101 p.60 ("\l", lower-case L)
}

var fallbackSymbol = symbolInfo{"APRS Station", "station"}

// lookupSymbol resolves a (table, code) pair to a display label/icon. Any
// table character other than the literal primary "/" is treated as the
// alternate table, which is also where overlay characters live.
func lookupSymbol(table, code byte) symbolInfo {
	symbolMap := alternateSymbols
	if table == '/' {
		symbolMap = primarySymbols
	}
	if info, ok := symbolMap[code]; ok {
		return info
	}
	return fallbackSymbol
}
