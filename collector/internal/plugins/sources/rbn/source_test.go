package rbn

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func testSource(t *testing.T) *Source {
	t.Helper()
	source, err := New(config.Source{ID: "test", Type: "hamradio.rbn", Login: "HB9ABC"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return source
}

// Lines captured live from telnet.reversebeacon.net:7000 on 2026-07-19.
func TestParseLiveSpotLines(t *testing.T) {
	source := testSource(t)
	cases := []struct {
		line    string
		txName  string
		band    string
		mode    string
	}{
		{"DX de HA8TKS-#: 14053.00  G4EDG/P        CW     7 dB  24 WPM  CQ      1528Z\r\n", "England", "20m", "CW"},
		{"DX de DG1ELE-#: 21082.00  US5WAU         CW    24 dB  31 WPM  CQ      1528Z\r\n", "Ukraine", "15m", "CW"},
	}
	for _, c := range cases {
		record, ok := source.ParseSpot(c.line)
		if !ok {
			t.Fatalf("live line rejected: %s", c.line)
		}
		var payload map[string]any
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["band"] != c.band || payload["mode"] != c.mode {
			t.Fatalf("band/mode wrong for %s: %v/%v", c.line, payload["band"], payload["mode"])
		}
		if record.Domain != "hamradio" {
			t.Fatalf("unexpected domain %q", record.Domain)
		}
	}
}

func TestParseRejectsChatter(t *testing.T) {
	source := testSource(t)
	for _, line := range []string{"Local users: 477\r\n", "Spot rate: 9/s (33,943/h)\r\n", "HB9ABC de RELAY 19-Jul-2026 15:28Z >\r\n", "\r\n"} {
		if _, ok := source.ParseSpot(line); ok {
			t.Fatalf("chatter accepted as spot: %q", line)
		}
	}
}

func TestLoginRequired(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "hamradio.rbn"}, slog.Default()); err == nil {
		t.Fatal("missing login must be rejected")
	}
}
