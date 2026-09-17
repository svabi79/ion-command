package ndbc

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleKeepsDARTThenBuoy(t *testing.T) {
	body, err := os.ReadFile("testdata/latest_obs.txt")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "n", Type: "weather.ndbc"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
	var first, second map[string]any
	_ = json.Unmarshal(records[0].Payload, &first)
	_ = json.Unmarshal(records[1].Payload, &second)
	if first["stationId"] != "32411" || first["buoyKind"] != "DART" {
		t.Fatalf("dart first: %v", first)
	}
	if second["stationId"] != "41002" || second["buoyKind"] != "buoy" {
		t.Fatalf("buoy second: %v", second)
	}
}
