package incidentnews

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleKeepsRecentGeocoded(t *testing.T) {
	body, _ := os.ReadFile("testdata/incidents.csv")
	source, err := New(config.Source{ID: "i", Type: "maritime.incidentnews"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.now = func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["incidentId"] != "11219" {
		t.Fatalf("%v", payload)
	}
}
