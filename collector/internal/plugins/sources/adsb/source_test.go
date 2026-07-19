package adsb

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

// testdata/point_live.json is a live api.adsb.lol v2/point capture around
// JN47 on 2026-07-19 (254 aircraft, 13 of them on the ground).
func TestSampleParsesLiveResponse(t *testing.T) {
	body, err := os.ReadFile("testdata/point_live.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "test", Type: "aviation.adsb"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 200 {
		t.Fatalf("expected the live fleet, got %d", len(records))
	}
	grounded := 0
	for _, record := range records {
		var payload map[string]any
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["onGround"].(bool) {
			grounded++
		}
		if record.Domain != "aviation" {
			t.Fatalf("unexpected domain %q", record.Domain)
		}
	}
	if grounded < 5 {
		t.Fatalf(`the "ground" string alt_baro trap must be handled, got %d grounded`, grounded)
	}
}

func TestPollFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "aviation.adsb", PollSeconds: 5}, slog.Default()); err == nil {
		t.Fatal("sub-ten-second poll must be rejected")
	}
}
