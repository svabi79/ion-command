// Package iso3166 maps ISO 3166-1 alpha-2/alpha-3 country codes to
// public-domain geographic centroids for globe placement. Coordinates are
// atlas approximations (Natural Earth style), not a third-party dashboard.
package iso3166

import "strings"

type centroid struct {
	Name string
	Lat  float64
	Lon  float64
}

// Lookup accepts an ISO 3166-1 alpha-2 or alpha-3 code (any case) and
// returns a WGS84 centroid. Unknown codes return ok=false.
func Lookup(code string) (lat, lon float64, name string, ok bool) {
	key := strings.ToUpper(strings.TrimSpace(code))
	if len(key) == 2 {
		key = alpha2to3[key]
	}
	entry, found := centroids[key]
	if !found || key == "" {
		return 0, 0, "", false
	}
	return entry.Lat, entry.Lon, entry.Name, true
}

var centroids = map[string]centroid{
	"AFG": {"Afghanistan", 33.9, 67.7}, "AGO": {"Angola", -11.2, 17.9}, "ALB": {"Albania", 41.2, 20.2},
	"ARE": {"United Arab Emirates", 23.4, 53.8}, "ARG": {"Argentina", -38.4, -63.6}, "ARM": {"Armenia", 40.1, 45.0},
	"AUS": {"Australia", -25.3, 133.8}, "AUT": {"Austria", 47.5, 14.6}, "AZE": {"Azerbaijan", 40.1, 47.6},
	"BDI": {"Burundi", -3.4, 29.9}, "BEL": {"Belgium", 50.5, 4.5}, "BEN": {"Benin", 9.3, 2.3},
	"BFA": {"Burkina Faso", 12.2, -1.6}, "BGD": {"Bangladesh", 23.7, 90.4}, "BGR": {"Bulgaria", 42.7, 25.5},
	"BHR": {"Bahrain", 26.0, 50.6}, "BHS": {"Bahamas", 25.0, -77.4}, "BIH": {"Bosnia and Herzegovina", 43.9, 17.7},
	"BLR": {"Belarus", 53.7, 27.9}, "BLZ": {"Belize", 17.2, -88.5}, "BOL": {"Bolivia", -16.3, -63.6},
	"BRA": {"Brazil", -14.2, -51.9}, "BRB": {"Barbados", 13.2, -59.5}, "BRN": {"Brunei", 4.5, 114.7},
	"BTN": {"Bhutan", 27.5, 90.4}, "BWA": {"Botswana", -22.3, 24.7}, "CAF": {"Central African Republic", 6.6, 20.9},
	"CAN": {"Canada", 56.1, -106.3}, "CHE": {"Switzerland", 46.8, 8.2}, "CHL": {"Chile", -35.7, -71.5},
	"CHN": {"China", 35.9, 104.2}, "CIV": {"Côte d'Ivoire", 7.5, -5.5}, "CMR": {"Cameroon", 7.4, 12.4},
	"COD": {"Democratic Republic of the Congo", -4.0, 21.8}, "COG": {"Congo", -0.2, 15.8}, "COL": {"Colombia", 4.6, -74.3},
	"COM": {"Comoros", -11.6, 43.3}, "CPV": {"Cabo Verde", 16.0, -24.0}, "CRI": {"Costa Rica", 9.7, -83.8},
	"CUB": {"Cuba", 21.5, -77.8}, "CYP": {"Cyprus", 35.1, 33.4}, "CZE": {"Czechia", 49.8, 15.5},
	"DEU": {"Germany", 51.2, 10.5}, "DJI": {"Djibouti", 11.8, 42.6}, "DNK": {"Denmark", 56.3, 9.5},
	"DOM": {"Dominican Republic", 18.7, -70.2}, "DZA": {"Algeria", 28.0, 1.7}, "ECU": {"Ecuador", -1.8, -78.2},
	"EGY": {"Egypt", 26.8, 30.8}, "ERI": {"Eritrea", 15.2, 39.8}, "ESH": {"Western Sahara", 24.2, -13.0},
	"ESP": {"Spain", 40.5, -3.7}, "EST": {"Estonia", 58.6, 25.0}, "ETH": {"Ethiopia", 9.1, 40.5},
	"FIN": {"Finland", 61.9, 25.7}, "FJI": {"Fiji", -17.7, 178.1}, "FRA": {"France", 46.2, 2.2},
	"GAB": {"Gabon", -0.8, 11.6}, "GBR": {"United Kingdom", 55.4, -3.4}, "GEO": {"Georgia", 42.3, 43.4},
	"GHA": {"Ghana", 7.9, -1.0}, "GIN": {"Guinea", 9.9, -9.7}, "GMB": {"Gambia", 13.4, -15.3},
	"GNB": {"Guinea-Bissau", 11.8, -15.2}, "GNQ": {"Equatorial Guinea", 1.7, 10.3}, "GRC": {"Greece", 39.1, 21.8},
	"GTM": {"Guatemala", 15.8, -90.2}, "GUY": {"Guyana", 4.9, -58.9}, "HND": {"Honduras", 15.2, -86.2},
	"HRV": {"Croatia", 45.1, 15.2}, "HTI": {"Haiti", 18.97, -72.3}, "HUN": {"Hungary", 47.2, 19.5},
	"IDN": {"Indonesia", -0.8, 113.9}, "IND": {"India", 20.6, 79.0}, "IRL": {"Ireland", 53.1, -8.2},
	"IRN": {"Iran", 32.4, 53.7}, "IRQ": {"Iraq", 33.2, 43.7}, "ISL": {"Iceland", 64.96, -19.0},
	"ISR": {"Israel", 31.0, 34.9}, "ITA": {"Italy", 41.9, 12.6}, "JAM": {"Jamaica", 18.1, -77.3},
	"JOR": {"Jordan", 30.6, 36.2}, "JPN": {"Japan", 36.2, 138.3}, "KAZ": {"Kazakhstan", 48.0, 66.9},
	"KEN": {"Kenya", 0.0, 37.9}, "KGZ": {"Kyrgyzstan", 41.2, 74.8}, "KHM": {"Cambodia", 12.6, 104.99},
	"KOR": {"South Korea", 35.9, 127.8}, "KWT": {"Kuwait", 29.3, 47.5}, "LAO": {"Laos", 19.9, 102.5},
	"LBN": {"Lebanon", 33.9, 35.9}, "LBR": {"Liberia", 6.4, -9.4}, "LBY": {"Libya", 26.3, 17.2},
	"LKA": {"Sri Lanka", 7.9, 80.8}, "LSO": {"Lesotho", -29.6, 28.2}, "LTU": {"Lithuania", 55.2, 23.9},
	"LUX": {"Luxembourg", 49.8, 6.1}, "LVA": {"Latvia", 56.9, 24.6}, "MAR": {"Morocco", 31.8, -7.1},
	"MDA": {"Moldova", 47.4, 28.4}, "MDG": {"Madagascar", -18.8, 46.9}, "MEX": {"Mexico", 23.6, -102.6},
	"MKD": {"North Macedonia", 41.6, 21.7}, "MLI": {"Mali", 17.6, -4.0}, "MLT": {"Malta", 35.9, 14.4},
	"MMR": {"Myanmar", 21.9, 95.96}, "MNE": {"Montenegro", 42.7, 19.4}, "MNG": {"Mongolia", 46.9, 103.8},
	"MOZ": {"Mozambique", -18.7, 35.5}, "MRT": {"Mauritania", 21.0, -10.9}, "MUS": {"Mauritius", -20.3, 57.6},
	"MWI": {"Malawi", -13.3, 34.3}, "MYS": {"Malaysia", 4.2, 101.98}, "NAM": {"Namibia", -22.96, 18.5},
	"NER": {"Niger", 17.6, 8.1}, "NGA": {"Nigeria", 9.1, 8.7}, "NIC": {"Nicaragua", 12.9, -85.2},
	"NLD": {"Netherlands", 52.1, 5.3}, "NOR": {"Norway", 60.5, 8.5}, "NPL": {"Nepal", 28.4, 84.1},
	"NZL": {"New Zealand", -40.9, 174.9}, "OMN": {"Oman", 21.5, 55.9}, "PAK": {"Pakistan", 30.4, 69.3},
	"PAN": {"Panama", 8.5, -80.8}, "PER": {"Peru", -9.2, -75.0}, "PHL": {"Philippines", 12.9, 121.8},
	"PNG": {"Papua New Guinea", -6.3, 143.96}, "POL": {"Poland", 51.9, 19.1}, "PRK": {"North Korea", 40.3, 127.5},
	"PRT": {"Portugal", 39.4, -8.2}, "PRY": {"Paraguay", -23.4, -58.4}, "PSE": {"Palestine", 31.95, 35.2},
	"QAT": {"Qatar", 25.4, 51.2}, "ROU": {"Romania", 45.9, 24.97}, "RUS": {"Russia", 61.5, 105.3},
	"RWA": {"Rwanda", -1.9, 29.9}, "SAU": {"Saudi Arabia", 23.9, 45.1}, "SDN": {"Sudan", 12.9, 30.2},
	"SEN": {"Senegal", 14.5, -14.5}, "SGP": {"Singapore", 1.4, 103.8}, "SLB": {"Solomon Islands", -9.6, 160.2},
	"SLE": {"Sierra Leone", 8.5, -11.8}, "SLV": {"El Salvador", 13.8, -88.9}, "SOM": {"Somalia", 5.2, 46.2},
	"SRB": {"Serbia", 44.0, 21.0}, "SSD": {"South Sudan", 6.9, 31.3}, "STP": {"Sao Tome and Principe", 0.2, 6.6},
	"SUR": {"Suriname", 3.9, -56.0}, "SVK": {"Slovakia", 48.7, 19.7}, "SVN": {"Slovenia", 46.2, 14.8},
	"SWE": {"Sweden", 60.1, 18.6}, "SWZ": {"Eswatini", -26.5, 31.5}, "SYR": {"Syria", 34.8, 39.0},
	"TCD": {"Chad", 15.5, 18.7}, "TGO": {"Togo", 8.6, 0.8}, "THA": {"Thailand", 15.9, 100.99},
	"TJK": {"Tajikistan", 38.9, 71.3}, "TKM": {"Turkmenistan", 38.97, 59.6}, "TLS": {"Timor-Leste", -8.9, 125.7},
	"TTO": {"Trinidad and Tobago", 10.7, -61.2}, "TUN": {"Tunisia", 33.9, 9.5}, "TUR": {"Türkiye", 38.96, 35.2},
	"TZA": {"Tanzania", -6.4, 34.9}, "UGA": {"Uganda", 1.4, 32.3}, "UKR": {"Ukraine", 48.4, 31.2},
	"URY": {"Uruguay", -32.5, -55.8}, "USA": {"United States", 37.1, -95.7}, "UZB": {"Uzbekistan", 41.4, 64.6},
	"VEN": {"Venezuela", 6.4, -66.6}, "VNM": {"Vietnam", 14.1, 108.3}, "YEM": {"Yemen", 15.6, 48.5},
	"ZAF": {"South Africa", -30.6, 22.9}, "ZMB": {"Zambia", -13.1, 27.8}, "ZWE": {"Zimbabwe", -19.0, 29.2},
	"XKX": {"Kosovo", 42.6, 20.9},
}

// ISO 3166-1 alpha-2 → alpha-3. Only codes that have a centroid above.
var alpha2to3 = map[string]string{
	"AF": "AFG", "AO": "AGO", "AL": "ALB", "AE": "ARE", "AR": "ARG", "AM": "ARM",
	"AU": "AUS", "AT": "AUT", "AZ": "AZE", "BI": "BDI", "BE": "BEL", "BJ": "BEN",
	"BF": "BFA", "BD": "BGD", "BG": "BGR", "BH": "BHR", "BS": "BHS", "BA": "BIH",
	"BY": "BLR", "BZ": "BLZ", "BO": "BOL", "BR": "BRA", "BB": "BRB", "BN": "BRN",
	"BT": "BTN", "BW": "BWA", "CF": "CAF", "CA": "CAN", "CH": "CHE", "CL": "CHL",
	"CN": "CHN", "CI": "CIV", "CM": "CMR", "CD": "COD", "CG": "COG", "CO": "COL",
	"KM": "COM", "CV": "CPV", "CR": "CRI", "CU": "CUB", "CY": "CYP", "CZ": "CZE",
	"DE": "DEU", "DJ": "DJI", "DK": "DNK", "DO": "DOM", "DZ": "DZA", "EC": "ECU",
	"EG": "EGY", "ER": "ERI", "EH": "ESH", "ES": "ESP", "EE": "EST", "ET": "ETH",
	"FI": "FIN", "FJ": "FJI", "FR": "FRA", "GA": "GAB", "GB": "GBR", "GE": "GEO",
	"GH": "GHA", "GN": "GIN", "GM": "GMB", "GW": "GNB", "GQ": "GNQ", "GR": "GRC",
	"GT": "GTM", "GY": "GUY", "HN": "HND", "HR": "HRV", "HT": "HTI", "HU": "HUN",
	"ID": "IDN", "IN": "IND", "IE": "IRL", "IR": "IRN", "IQ": "IRQ", "IS": "ISL",
	"IL": "ISR", "IT": "ITA", "JM": "JAM", "JO": "JOR", "JP": "JPN", "KZ": "KAZ",
	"KE": "KEN", "KG": "KGZ", "KH": "KHM", "KR": "KOR", "KW": "KWT", "LA": "LAO",
	"LB": "LBN", "LR": "LBR", "LY": "LBY", "LK": "LKA", "LS": "LSO", "LT": "LTU",
	"LU": "LUX", "LV": "LVA", "MA": "MAR", "MD": "MDA", "MG": "MDG", "MX": "MEX",
	"MK": "MKD", "ML": "MLI", "MT": "MLT", "MM": "MMR", "ME": "MNE", "MN": "MNG",
	"MZ": "MOZ", "MR": "MRT", "MU": "MUS", "MW": "MWI", "MY": "MYS", "NA": "NAM",
	"NE": "NER", "NG": "NGA", "NI": "NIC", "NL": "NLD", "NO": "NOR", "NP": "NPL",
	"NZ": "NZL", "OM": "OMN", "PK": "PAK", "PA": "PAN", "PE": "PER", "PH": "PHL",
	"PG": "PNG", "PL": "POL", "KP": "PRK", "PT": "PRT", "PY": "PRY", "PS": "PSE",
	"QA": "QAT", "RO": "ROU", "RU": "RUS", "RW": "RWA", "SA": "SAU", "SD": "SDN",
	"SN": "SEN", "SG": "SGP", "SB": "SLB", "SL": "SLE", "SV": "SLV", "SO": "SOM",
	"RS": "SRB", "SS": "SSD", "ST": "STP", "SR": "SUR", "SK": "SVK", "SI": "SVN",
	"SE": "SWE", "SZ": "SWZ", "SY": "SYR", "TD": "TCD", "TG": "TGO", "TH": "THA",
	"TJ": "TJK", "TM": "TKM", "TL": "TLS", "TT": "TTO", "TN": "TUN", "TR": "TUR",
	"TZ": "TZA", "UG": "UGA", "UA": "UKR", "UY": "URY", "US": "USA", "UZ": "UZB",
	"VE": "VEN", "VN": "VNM", "YE": "YEM", "ZA": "ZAF", "ZM": "ZMB", "ZW": "ZWE",
	"XK": "XKX",
}
