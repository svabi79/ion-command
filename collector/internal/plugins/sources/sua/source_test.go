package sua

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleRanksRestrictedBeforeAlert(t *testing.T) {
	body, _ := os.ReadFile("testdata/sua.json")
	source, err := New(config.Source{ID: "s", Type: "aviation.sua"}, slog.Default())
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
	if payload["title"] != "R-4001" || payload["kind"] != "airspace" {
		t.Fatalf("%v", payload)
	}
}
