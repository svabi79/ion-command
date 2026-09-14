package powerplants

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleDropsSmallPlants(t *testing.T) {
	body, _ := os.ReadFile("testdata/plants.csv")
	source, err := New(config.Source{ID: "p", Type: "geography.powerplants", PollSeconds: 86400}, slog.Default())
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
	if payload["name"] != "Big Plant" {
		t.Fatalf("%v", payload)
	}
}
