package nhc

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleKeepsNamedCentre(t *testing.T) {
	body, err := os.ReadFile("testdata/current.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "n", Type: "weather.nhc"}, slog.Default())
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
	if payload["name"] != "Norbert" || payload["classLabel"] != "Tropical Storm" {
		t.Fatalf("%v", payload)
	}
	if payload["longitude"].(float64) != -126.6 {
		t.Fatalf("lon %v", payload["longitude"])
	}
}

func TestEmptyStorms(t *testing.T) {
	source, err := New(config.Source{ID: "n", Type: "weather.nhc"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context) ([]byte, error) { return []byte(`{"activeStorms":[]}`), nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 0 {
		t.Fatalf("%v %d", err, len(records))
	}
}

func TestPollFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "weather.nhc", PollSeconds: 60}, slog.Default()); err == nil {
		t.Fatal("expected floor rejection")
	}
}
