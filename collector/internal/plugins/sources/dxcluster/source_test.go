package dxcluster

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func testSource(t *testing.T) *Source {
	t.Helper()
	source, err := New(config.Source{ID: "test", Type: "hamradio.dxcluster", Login: "HB9ABC", Broker: "dxc.example:7373"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return source
}

type decodedSpot struct {
	SpotID      string  `json:"spotId"`
	TXCallsign  string  `json:"txCallsign"`
	RXCallsign  string  `json:"rxCallsign"`
	TXLongitude float64 `json:"txLongitude"`
	TXLatitude  float64 `json:"txLatitude"`
	RXLongitude float64 `json:"rxLongitude"`
	RXLatitude  float64 `json:"rxLatitude"`
	FrequencyHz int64   `json:"frequencyHz"`
	Band        string  `json:"band"`
	Mode        string  `json:"mode"`
	SNRDb       *int    `json:"snrDb"`
	TXRegion    string  `json:"txRegion"`
	RXRegion    string  `json:"rxRegion"`
}

// Lines captured live from dxc.nc7j.com:7373 and k1ttt.net:7373 on 2026-08-28
// (placeholder callsign N0CALL). NC7J relays RBN skimmer spots through its DX
// cluster feed (hence the "-#" spotter suffix and the fixed CW/dB/WPM shape);
// K1TTT gave genuine human-typed spots with free-text comments.
func TestParseLiveSpotLines(t *testing.T) {
	source := testSource(t)
	cases := []struct {
		name           string
		line           string
		txCall, rxCall string
		band, mode     string
		wantSNR        *int
	}{
		{
			name:   "RBN-relayed CW spot with signal report",
			line:   "DX de OH6BG-#:   14060.0  GW4ZUA       CW 11 dB 15 WPM CQ             1550Z\r\n",
			txCall: "GW4ZUA", rxCall: "OH6BG", band: "20m", mode: "CW", wantSNR: intPtr(11),
		},
		{
			// Spotter carries both a personal node suffix ("-1") and the
			// RBN relay marker ("-#") - the non-greedy callsign group must
			// still stop at the right place.
			name:   "spotter with node suffix and relay marker",
			line:   "DX de K3PA-1-#:  14050.0  AB2AX        CW 7 dB 9 WPM CQ           WNY 1550Z\r\n",
			txCall: "AB2AX", rxCall: "K3PA-1", band: "20m", mode: "CW", wantSNR: intPtr(7),
		},
		{
			name:   "bare human spot with no comment at all",
			line:   "DX de RU3GC:     21240.0  9M26MA                                      1550Z\r\n",
			txCall: "9M26MA", rxCall: "RU3GC", band: "15m", mode: "", wantSNR: nil,
		},
		{
			// "59+15dB" is an S-meter callout glued to the RST, not a space-
			// delimited "N dB" signal report, and must not be misread as one.
			name:   "combined RST and S-meter callout is not an SNR",
			line:   "DX de F4GCU:     14242.0  EI4IT        59+15dB Tnx!                   1550Z\r\n",
			txCall: "EI4IT", rxCall: "F4GCU", band: "20m", mode: "", wantSNR: nil,
		},
		{
			name:   "auto-spotted FT8 decode with negative SNR",
			line:   "DX de HA5WV:     28074.0  YB4KAR       FT8 -7 dB 1706 Hz TU 73        1550Z\r\n",
			txCall: "YB4KAR", rxCall: "HA5WV", band: "10m", mode: "FT8", wantSNR: intPtr(-7),
		},
		{
			name:   "beacon spot with a /B suffix DX callsign",
			line:   "DX de OE9GHV-#:  10144.2  G0MBA/B      CW 9 dB 18 WPM BEACON          1550Z\r\n",
			txCall: "G0MBA/B", rxCall: "OE9GHV", band: "30m", mode: "CW", wantSNR: intPtr(9),
		},
		{
			name:   "K1TTT special event station, no signal report",
			line:   "DX de PD0YL:     14262.0  PD00DOG      SES INT DD                     1550Z\r\n",
			txCall: "PD00DOG", rxCall: "PD0YL", band: "20m", mode: "", wantSNR: nil,
		},
		{
			name:   "K1TTT auto-spotted FT8 decode",
			line:   "DX de UA3ARC:    21074.0  N1IG         FT8 -15 dB 1503 Hz             1550Z\r\n",
			txCall: "N1IG", rxCall: "UA3ARC", band: "15m", mode: "FT8", wantSNR: intPtr(-15),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			record, ok := source.ParseSpot(c.line)
			if !ok {
				t.Fatalf("live line rejected: %s", c.line)
			}
			var spot decodedSpot
			if err := json.Unmarshal(record.Payload, &spot); err != nil {
				t.Fatal(err)
			}
			if spot.TXCallsign != c.txCall || spot.RXCallsign != c.rxCall {
				t.Fatalf("callsigns: got tx=%q rx=%q, want tx=%q rx=%q", spot.TXCallsign, spot.RXCallsign, c.txCall, c.rxCall)
			}
			if spot.Band != c.band || spot.Mode != c.mode {
				t.Fatalf("band/mode wrong for %s: got %v/%v, want %v/%v", c.line, spot.Band, spot.Mode, c.band, c.mode)
			}
			if (spot.SNRDb == nil) != (c.wantSNR == nil) || (spot.SNRDb != nil && *spot.SNRDb != *c.wantSNR) {
				t.Fatalf("snrDb wrong for %s: got %v, want %v", c.line, snrString(spot.SNRDb), snrString(c.wantSNR))
			}
			if record.Domain != "hamradio" {
				t.Fatalf("unexpected domain %q", record.Domain)
			}
			if spot.TXRegion == "" || spot.RXRegion == "" {
				t.Fatalf("both ends should resolve a region via the country file: %#v", spot)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

func snrString(v *int) string {
	if v == nil {
		return "<nil>"
	}
	return strconv.Itoa(*v)
}

func TestParseRejectsChatter(t *testing.T) {
	source := testSource(t)
	// Captured verbatim from the same live sessions: banners, prompts and
	// server notices that must never be mistaken for a spot line.
	lines := []string{
		"Hello  N0CALL\r\n",
		"Utah DX Cluster and CW/RTTY Skimmer Server\r\n",
		"N0CALL de NC7J 28-Aug 1550Z arc6>\r\n",
		"Sorry N0CALL but you are already connected to 3 other nodes (on WA9PIE-2,GB7BAA,PI1LAP-1)\r\n",
		"N0CALL is not a valid callsign\r\n",
		"Please enter your call: \r\n",
		"\r\n",
	}
	for _, line := range lines {
		if _, ok := source.ParseSpot(line); ok {
			t.Fatalf("chatter accepted as spot: %q", line)
		}
	}
}

func TestParseSkipsUnplaceableCallsigns(t *testing.T) {
	source := testSource(t)
	// "QRZQRZ" is not an assigned DXCC prefix (verified directly against the
	// cty package) - it must never be placed at an invented position.
	cases := []string{
		"DX de HB9ABC-#:  14060.0  QRZQRZ       CW 10 dB 20 WPM CQ             1550Z\r\n",
		"DX de QRZQRZ-#:  14060.0  HB9ABC       CW 10 dB 20 WPM CQ             1550Z\r\n",
	}
	for _, line := range cases {
		if _, ok := source.ParseSpot(line); ok {
			t.Fatalf("spot with an unresolvable callsign must be skipped: %q", line)
		}
	}
}

func TestParseSkipsUnrecognisedBand(t *testing.T) {
	source := testSource(t)
	// 13.555 MHz is real, observed WSPR/experimental traffic (verified live
	// against wspr.live) but is not inside any recognised amateur band, so a
	// spot there must be skipped rather than assigned a fabricated band.
	line := "DX de HB9ABC-#:  13555.4  W3PM         CW 10 dB 20 WPM CQ             1550Z\r\n"
	if _, ok := source.ParseSpot(line); ok {
		t.Fatalf("spot outside any recognised amateur band must be skipped: %q", line)
	}
}

func TestLoginRequired(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "hamradio.dxcluster", Broker: "dxc.example:7373"}, slog.Default()); err == nil {
		t.Fatal("missing login must be rejected")
	}
}

func TestBrokerRequired(t *testing.T) {
	// Unlike RBN, there is no single canonical DX cluster node, so - unlike
	// RBN - a missing address must be rejected rather than silently defaulted.
	if _, err := New(config.Source{ID: "x", Type: "hamradio.dxcluster", Login: "HB9ABC"}, slog.Default()); err == nil {
		t.Fatal("missing broker address must be rejected")
	}
}

func TestWrongTypeRejected(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "hamradio.rbn", Login: "HB9ABC", Broker: "dxc.example:7373"}, slog.Default()); err == nil {
		t.Fatal("wrong source type must be rejected")
	}
}
