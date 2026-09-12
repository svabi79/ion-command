package pads

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleKeepsEarthPadsWithCoordinates(t *testing.T) {
	body, err := os.ReadFile("testdata/locations.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "p", Type: "space.pads", PollSeconds: 21600}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "pad" || payload["name"] != "Kennedy Space Center, USA" {
		t.Fatalf("%v", payload)
	}
}

func TestPollFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "space.pads", PollSeconds: 60}, slog.Default()); err == nil {
		t.Fatal("expected floor rejection")
	}
}
