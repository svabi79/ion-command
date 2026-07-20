package opensky

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

// The fixture is a trimmed live snapshot from opensky-network.org
// (states/all?extended=1) including a rotorcraft (category 8), an on-ground
// state, and a synthetic stale entry that the position age gate must drop.
func TestSampleParsesLiveSnapshot(t *testing.T) {
	body, err := os.ReadFile("testdata/states_live.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	source, err := New(config.Source{ID: "test", Type: "aviation.opensky", Enabled: true}, slog.Default())
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatalf("sample: %v", err)
	}
	// Fixture ships 7 states; the fabricated stale one (icao deadbe, 999 s
	// old fix) must be filtered. NOTE: the age gate compares against the
	// fixture's own snapshot time, so the test stays valid forever.
	if len(records) != 6 {
		t.Fatalf("expected 6 records, got %d", len(records))
	}
	sawHelicopter := false
	sawGround := false
	for _, record := range records {
		var payload map[string]any
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			t.Fatalf("payload decode: %v", err)
		}
		if payload["hex"] == "deadbe" {
			t.Fatalf("stale state must be dropped")
		}
		if record.Domain != "aviation" {
			t.Fatalf("unexpected domain %q", record.Domain)
		}
		if payload["kind"] == "helicopter" {
			sawHelicopter = true
		}
		if payload["onGround"] == true {
			sawGround = true
		}
		if payload["validSeconds"].(float64) < 300 {
			t.Fatalf("validSeconds must cover the slow poll, got %v", payload["validSeconds"])
		}
		if _, ok := payload["lat"].(float64); !ok {
			t.Fatalf("lat missing")
		}
	}
	if !sawHelicopter {
		t.Fatalf("category 8 must map to helicopter")
	}
	if !sawGround {
		t.Fatalf("on-ground state expected in fixture")
	}
}

func TestPollFloorRejected(t *testing.T) {
	_, err := New(config.Source{ID: "x", Type: "aviation.opensky", PollSeconds: 60}, slog.Default())
	if err == nil {
		t.Fatalf("expected poll floor rejection")
	}
}
