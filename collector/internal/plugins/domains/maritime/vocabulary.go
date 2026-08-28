package maritime

import (
	"fmt"
	"strings"
	"time"
)

// validityFor returns how long a position fix should be considered current
// on the globe. Grounded in the ITU-R M.1371 nominal reporting-interval
// table: a Class A vessel under way reports every 2-10 seconds and a Class B
// vessel under way roughly every 30 seconds, but either class drops to once
// every three minutes at anchor, moored, or aground. The window is generous
// relative to the nominal interval so a marker survives a couple of missed
// reports or provider-side gaps without flickering, while a vessel that
// truly left the subscribed area still ages out at a sensible horizon.
func validityFor(stationary bool) time.Duration {
	if stationary {
		return 20 * time.Minute
	}
	return 5 * time.Minute
}

// isStationaryStatus reports whether a Class A navigational-status code
// describes a vessel AIS itself only expects to hear from roughly every
// three minutes, rather than every few seconds (ITU-R M.1371's own
// reporting-interval table draws exactly this line). Status 15 ("not
// defined") is deliberately excluded: an unset status says nothing about
// motion, so it must not be treated as if the vessel were stationary.
func isStationaryStatus(navStatus int) bool {
	switch navStatus {
	case 1, 5, 6: // at anchor, moored, aground
		return true
	default:
		return false
	}
}

// navStatusNames are the sixteen AIS navigational-status codes (ITU-R
// M.1371, the field carried in message types 1/2/3 and 5). Code 15 is both
// the explicit "not defined" value and this table's out-of-range fallback.
var navStatusNames = map[int]string{
	0:  "Under way using engine",
	1:  "At anchor",
	2:  "Not under command",
	3:  "Restricted manoeuvrability",
	4:  "Constrained by draught",
	5:  "Moored",
	6:  "Aground",
	7:  "Engaged in fishing",
	8:  "Under way sailing",
	9:  "Reserved (high-speed craft)",
	10: "Reserved (wing in ground)",
	11: "Power-driven vessel towing astern",
	12: "Power-driven vessel pushing ahead or towing alongside",
	13: "Reserved",
	14: "AIS-SART, MOB or EPIRB active",
	15: "Not defined",
}

func navStatusText(code int) string {
	if name, ok := navStatusNames[code]; ok {
		return name
	}
	return "Not defined"
}

// shipTypeVocabulary maps the AIS ship/cargo type code (ITU-R M.1371 Table
// 53, carried as ShipStaticData.Type / StaticDataReport ReportB.ShipType) to
// a human-readable category and a generic icon slug for the renderer. Code 0
// and any code this table does not recognise both mean "not available" —
// the category comes back empty and the icon falls back to a plain vessel.
func shipTypeVocabulary(code int) (category, icon string) {
	switch {
	case code == 0:
		return "", "vessel"
	case code >= 20 && code <= 29:
		return "Wing in Ground", "vessel-wig"
	case code == 30:
		return "Fishing", "vessel-fishing"
	case code == 31 || code == 32:
		return "Towing", "vessel-tug"
	case code == 33:
		return "Dredging", "vessel-dredging"
	case code == 34:
		return "Diving Operations", "vessel-diving"
	case code == 35:
		return "Military", "vessel-military"
	case code == 36:
		return "Sailing", "vessel-sailing"
	case code == 37:
		return "Pleasure Craft", "vessel-pleasure"
	case code >= 40 && code <= 49:
		return "High-Speed Craft", "vessel-hsc"
	case code == 50:
		return "Pilot Vessel", "vessel-pilot"
	case code == 51:
		return "Search and Rescue", "vessel-sar"
	case code == 52:
		return "Tug", "vessel-tug"
	case code == 53:
		return "Port Tender", "vessel-tender"
	case code == 54:
		return "Anti-Pollution Equipment", "vessel-other"
	case code == 55:
		return "Law Enforcement", "vessel-law"
	case code == 58:
		return "Medical Transport", "vessel-medical"
	case code == 59:
		return "Noncombatant Ship", "vessel-other"
	case code >= 60 && code <= 69:
		return "Passenger", "vessel-passenger"
	case code >= 70 && code <= 79:
		return "Cargo", "vessel-cargo"
	case code >= 80 && code <= 89:
		return "Tanker", "vessel-tanker"
	case code >= 90 && code <= 99:
		return "Other", "vessel-other"
	default:
		return "", "vessel"
	}
}

// mmsiCategory classifies an MMSI by its ITU-R M.1371 numbering-plan prefix.
// AIS multiplexes several kinds of participant onto the same message
// transport and MMSI address space; only a genuine ship station (leading
// digit 2-7) is a "vessel" for this domain's purposes. See
// https://www.navcen.uscg.gov/mmsi-formats for the numbering plan this
// mirrors.
func mmsiCategory(mmsi int64) string {
	if mmsi < 1 || mmsi > 999999999 {
		return "invalid"
	}
	digits := fmt.Sprintf("%09d", mmsi)
	switch {
	case strings.HasPrefix(digits, "111"):
		return "sar_aircraft"
	case strings.HasPrefix(digits, "99"):
		return "aid_to_navigation"
	case strings.HasPrefix(digits, "98"):
		return "auxiliary_craft"
	case strings.HasPrefix(digits, "970"), strings.HasPrefix(digits, "972"), strings.HasPrefix(digits, "974"):
		return "sar_transmitter" // AIS-SART / MOB device / EPIRB-AIS
	case strings.HasPrefix(digits, "97"):
		return "diver_radio"
	case strings.HasPrefix(digits, "00"):
		return "base_station"
	case strings.HasPrefix(digits, "0"):
		return "group_ship" // 0MIDxxxxxx group call, not an individual vessel
	case digits[0] >= '2' && digits[0] <= '7':
		return "vessel"
	default:
		return "unknown"
	}
}
