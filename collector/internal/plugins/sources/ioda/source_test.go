package ioda

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleDropsNormal(t *testing.T) {
	body, _ := os.ReadFile("testdata/alerts.json")
	source, err := New(config.Source{ID: "i", Type: "geography.ioda"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.now = func() time.Time { return time.Unix(1789411446, 0).UTC() }
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["level"] != "critical" || payload["country"] != "CO" {
		t.Fatalf("%v", payload)
	}
}
