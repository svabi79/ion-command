package ourairports

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleFiltersTypes(t *testing.T) {
	body, _ := os.ReadFile("testdata/airports.csv")
	source, err := New(config.Source{ID: "o", Type: "aviation.ourairports"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["iata"] != "JFK" {
		t.Fatalf("%v", payload)
	}
}
