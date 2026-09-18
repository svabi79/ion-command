package meteoalarm

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleKeepsCapPolygon(t *testing.T) {
	body, err := os.ReadFile("testdata/switzerland.atom")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "m", Type: "weather.meteoalarm"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.countries = []string{"switzerland"}
	source.now = func() time.Time { return time.Date(2026, 9, 18, 6, 0, 0, 0, time.UTC) }
	source.fetch = func(context.Context, string) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["event"] != "Heavy thunderstorm" || payload["kind"] != "alert" {
		t.Fatalf("%v", payload)
	}
	if payload["alertId"] != "demo-alert-1" {
		t.Fatalf("id %v", payload["alertId"])
	}
}

func TestCapPolygonLonLatOrder(t *testing.T) {
	ring := parseCapPolygon("45.9,8.9 45.9,9.1 46.0,9.1 45.9,8.9")
	if len(ring) < 4 || ring[0][0] != 8.9 || ring[0][1] != 45.9 {
		t.Fatalf("%v", ring)
	}
}
