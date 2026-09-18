package sondehub

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleDropsStaleSondes(t *testing.T) {
	body, err := os.ReadFile("testdata/sondes.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "s", Type: "weather.sondehub"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.now = func() time.Time { return time.Date(2026, 9, 18, 5, 0, 0, 0, time.UTC) }
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["kind"] != "sonde" || payload["serial"] != "Y3322142" {
		t.Fatalf("%v", payload)
	}
}
