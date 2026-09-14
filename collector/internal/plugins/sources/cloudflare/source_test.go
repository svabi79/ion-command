package cloudflare

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestRequiresToken(t *testing.T) {
	if _, err := New(config.Source{ID: "c", Type: "geography.cloudflare"}, slog.Default()); err == nil {
		t.Fatal("expected fail-closed without apiKey")
	}
}

func TestSampleMapsISO(t *testing.T) {
	body, _ := os.ReadFile("testdata/outages.json")
	source, err := New(config.Source{ID: "c", Type: "geography.cloudflare", ApiKey: "tok"}, slog.Default())
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
	if payload["country"] != "KE" || payload["latitude"] == 0 {
		t.Fatalf("%v", payload)
	}
}
