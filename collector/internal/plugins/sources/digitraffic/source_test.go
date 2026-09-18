package digitraffic

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleOpenFaultsAndDirways(t *testing.T) {
	faults, err := os.ReadFile("testdata/faults.json")
	if err != nil {
		t.Fatal(err)
	}
	dirways, err := os.ReadFile("testdata/dirways.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "d", Type: "maritime.digitraffic"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetchFaults = func(context.Context) ([]byte, error) { return faults, nil }
	source.fetchDirways = func(context.Context) ([]byte, error) { return dirways, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
	var first, second map[string]any
	_ = json.Unmarshal(records[0].Payload, &first)
	_ = json.Unmarshal(records[1].Payload, &second)
	if first["kind"] != "fault" || first["name"] != "Merikari" {
		t.Fatalf("fault %v", first)
	}
	if second["kind"] != "dirway" {
		t.Fatalf("dirway %v", second)
	}
}

func TestEmptyDirwaysSkippedQuietly(t *testing.T) {
	faults, _ := os.ReadFile("testdata/faults.json")
	empty, _ := os.ReadFile("testdata/dirways-empty.json")
	source, err := New(config.Source{ID: "d", Type: "maritime.digitraffic"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetchFaults = func(context.Context) ([]byte, error) { return faults, nil }
	source.fetchDirways = func(context.Context) ([]byte, error) { return empty, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
}
