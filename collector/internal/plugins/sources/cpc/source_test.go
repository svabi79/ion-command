package cpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleSkipsNormal(t *testing.T) {
	temp, _ := os.ReadFile("testdata/temp.json")
	precip, _ := os.ReadFile("testdata/precip.json")
	source, err := New(config.Source{ID: "c", Type: "weather.cpc"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context, rawURL string) ([]byte, error) {
		if strings.Contains(rawURL, "MapServer/1") || strings.Contains(rawURL, "precip") {
			return precip, nil
		}
		return temp, nil
	}
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
	var first, second map[string]any
	_ = json.Unmarshal(records[0].Payload, &first)
	_ = json.Unmarshal(records[1].Payload, &second)
	if first["kind"] != "outlook" || !strings.Contains(strings.ToLower(fmtString(first["label"])), "above") {
		t.Fatalf("temp %v", first)
	}
	if !strings.Contains(strings.ToLower(fmtString(second["label"])), "below") {
		t.Fatalf("precip %v", second)
	}
}

func fmtString(value any) string {
	text, _ := value.(string)
	return text
}
