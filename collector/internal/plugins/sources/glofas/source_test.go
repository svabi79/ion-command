package glofas

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleReadsShapefileZip(t *testing.T) {
	body, err := os.ReadFile("testdata/rfm.zip")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "g", Type: "weather.glofas"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.now = func() time.Time { return time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC) }
	source.fetch = func(context.Context, string) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) == 0 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["kind"] != "flood" {
		t.Fatalf("%v", payload)
	}
}
