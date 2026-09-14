package spc

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleOutlook(t *testing.T) {
	body, _ := os.ReadFile("testdata/day1.json")
	source, err := New(config.Source{ID: "s", Type: "weather.spc"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.urls = []string{"day1"}
	source.fetch = func(context.Context, string) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["label"] != "General Thunderstorms Risk" {
		t.Fatalf("%v", payload)
	}
}
