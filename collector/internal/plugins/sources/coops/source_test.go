package coops

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleSkipsStationsWithoutCoordinates(t *testing.T) {
	body, err := os.ReadFile("testdata/stations.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "c", Type: "weather.coops"}, slog.Default())
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
	if payload["kind"] != "tide" || payload["stationId"] != "8454000" {
		t.Fatalf("%v", payload)
	}
}
