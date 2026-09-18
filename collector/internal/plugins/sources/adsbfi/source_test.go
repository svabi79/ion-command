package adsbfi

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleMilitaryOnly(t *testing.T) {
	body, err := os.ReadFile("testdata/mil.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "a", Type: "aviation.adsbfi"}, slog.Default())
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
	if payload["hex"] != "ae57d5" || payload["kind"] != "aircraft" {
		t.Fatalf("%v", payload)
	}
}

func TestRefusesCivilSnapshotBroker(t *testing.T) {
	_, err := New(config.Source{ID: "a", Type: "aviation.adsbfi", Broker: "https://opendata.adsb.fi/api/v2/snapshot"}, slog.Default())
	if err == nil {
		t.Fatal("expected civil snapshot to be refused")
	}
}
