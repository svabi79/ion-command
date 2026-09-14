package nws

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleKeepsPolygonAlerts(t *testing.T) {
	body, err := os.ReadFile("testdata/alerts.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "n", Type: "weather.nws"}, slog.Default())
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
	if payload["event"] != "Tornado Warning" || payload["kind"] != "alert" {
		t.Fatalf("%v", payload)
	}
}
