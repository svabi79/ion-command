package usgs

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

// testdata/all_hour_live.json is a live capture of the USGS all_hour feed
// from 2026-07-19 (7 events, first one near Willow, Alaska).
func liveSource(t *testing.T) *Source {
	t.Helper()
	body, err := os.ReadFile("testdata/all_hour_live.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "test", Type: "earthquake.usgs"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context) ([]byte, error) { return body, nil }
	return source
}

func TestSampleParsesLiveFeed(t *testing.T) {
	source := liveSource(t)
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 7 {
		t.Fatalf("expected 7 events from the live fixture, got %d", len(records))
	}
	var payload map[string]any
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["quakeId"] != "aka2026oesaet" {
		t.Fatalf("unexpected first event: %v", payload["quakeId"])
	}
	lat := payload["latitude"].(float64)
	lon := payload["longitude"].(float64)
	if lat < 61.0 || lat > 62.5 || lon > -149.0 || lon < -150.5 {
		t.Fatalf("event position off (expected Alaska): %v/%v", lat, lon)
	}
	if records[0].Domain != "geophysics" {
		t.Fatalf("unexpected domain %q", records[0].Domain)
	}
}

func TestSampleDedupesUnchangedEvents(t *testing.T) {
	source := liveSource(t)
	if first, _ := source.sample(context.Background()); len(first) != 7 {
		t.Fatalf("first sample unexpected")
	}
	second, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("unchanged events must not repeat, got %d", len(second))
	}
}
