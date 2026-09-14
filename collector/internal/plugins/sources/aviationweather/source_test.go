package aviationweather

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleSIGMETPolygonSkipsOpenContour(t *testing.T) {
	body, _ := os.ReadFile("testdata/hazards.json")
	source, err := New(config.Source{ID: "a", Type: "aviation.aviationweather"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetchSigmet = func(context.Context) ([]byte, error) { return body, nil }
	source.fetchAirmet = func(context.Context) ([]byte, error) { return []byte(`{"features":[]}`), nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["kind"] != "sigmet" || payload["hazard"] != "TS" {
		t.Fatalf("%v", payload)
	}
}
