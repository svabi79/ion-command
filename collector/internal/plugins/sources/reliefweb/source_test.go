package reliefweb

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestRequiresAppname(t *testing.T) {
	if _, err := New(config.Source{ID: "r", Type: "humanitarian.reliefweb"}, slog.Default()); err == nil {
		t.Fatal("expected fail-closed without apiKey")
	}
}

func TestPollFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "r", Type: "humanitarian.reliefweb", ApiKey: "app", PollSeconds: 60}, slog.Default()); err == nil {
		t.Fatal("expected floor rejection")
	}
}

func TestSampleSkipsUnlocated(t *testing.T) {
	body, err := os.ReadFile("testdata/disasters.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "r", Type: "humanitarian.reliefweb", ApiKey: "test-app"}, slog.Default())
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
	if records[0].OriginalID != "reliefweb-50599" || payload["glide"] != "EQ-2025-000123-AFG" {
		t.Fatalf("%s %v", records[0].OriginalID, payload)
	}
	if payload["kind"] != "disaster" || payload["category"] != "Earthquake" {
		t.Fatalf("%v", payload)
	}
	if payload["latitude"].(float64) != 33.94 || payload["longitude"].(float64) != 67.71 {
		t.Fatalf("coords %v", payload)
	}
}
