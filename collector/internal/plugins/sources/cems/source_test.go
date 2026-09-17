package cems

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleSkipsClosed(t *testing.T) {
	listBody, _ := os.ReadFile("testdata/list.json")
	detailBody, _ := os.ReadFile("testdata/detail.json")
	source, err := New(config.Source{ID: "c", Type: "humanitarian.cems"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context, rawURL string) ([]byte, error) {
		if strings.Contains(rawURL, "EMSR929") || strings.Contains(rawURL, "code=") {
			return detailBody, nil
		}
		return listBody, nil
	}
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["activationId"] != "EMSR929" || payload["kind"] != "activation" {
		t.Fatalf("%v", payload)
	}
}
