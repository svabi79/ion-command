package ucdp

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestRequiresToken(t *testing.T) {
	if _, err := New(config.Source{ID: "u", Type: "conflict.ucdp"}, slog.Default()); err == nil {
		t.Fatal("expected fail-closed without apiKey")
	}
}

func TestSample(t *testing.T) {
	body, _ := os.ReadFile("testdata/ged.json")
	source, err := New(config.Source{ID: "u", Type: "conflict.ucdp", ApiKey: "token"}, slog.Default())
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
	if payload["fatalities"].(float64) != 3 || payload["eventType"] != "state-based" {
		t.Fatalf("%v", payload)
	}
}
