package gfw

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestRequiresToken(t *testing.T) {
	if _, err := New(config.Source{ID: "g", Type: "maritime.gfw"}, slog.Default()); err == nil {
		t.Fatal("expected fail-closed without apiKey")
	}
}

func TestSample(t *testing.T) {
	body, _ := os.ReadFile("testdata/events.json")
	source, err := New(config.Source{ID: "g", Type: "maritime.gfw", ApiKey: "tok"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["hours"].(float64) != 4.5 {
		t.Fatalf("%v", payload)
	}
}
