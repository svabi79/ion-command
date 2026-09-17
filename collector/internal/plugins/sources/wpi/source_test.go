package wpi

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleKeepsLargeThenMedium(t *testing.T) {
	body, _ := os.ReadFile("testdata/ports.csv")
	source, err := New(config.Source{ID: "w", Type: "maritime.wpi"}, slog.Default())
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
	if payload["name"] != "Maurer" || payload["portId"] != "7950" {
		t.Fatalf("%v", payload)
	}
}
