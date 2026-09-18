package bcwildfire

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSamplePerimeter(t *testing.T) {
	body, err := os.ReadFile("testdata/perimeters.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "b", Type: "wildfire.bc"}, slog.Default())
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
	if payload["kind"] != "perimeter" || payload["name"] != "Demo Fire" {
		t.Fatalf("%v", payload)
	}
}
