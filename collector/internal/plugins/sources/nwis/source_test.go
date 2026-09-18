package nwis

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleMergesFlowAndStage(t *testing.T) {
	iv, err := os.ReadFile("testdata/iv.json")
	if err != nil {
		t.Fatal(err)
	}
	rdb, err := os.ReadFile("testdata/sites.rdb")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "n", Type: "weather.nwis"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.sites = rankSites(parseSiteRDB(rdb))
	source.fetchIV = func(context.Context, string) ([]byte, error) { return iv, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["kind"] != "gauge" || payload["stationId"] != "01646500" {
		t.Fatalf("%v", payload)
	}
	if payload["streamflowCfs"] != 4200.0 {
		t.Fatalf("flow %v", payload["streamflowCfs"])
	}
}

func TestParseSiteRDB(t *testing.T) {
	body, _ := os.ReadFile("testdata/sites.rdb")
	sites := parseSiteRDB(body)
	if len(sites) != 2 || sites[0].ID != "01646500" {
		t.Fatalf("%v", sites)
	}
}
