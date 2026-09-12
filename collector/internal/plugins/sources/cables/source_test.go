package cables

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

func TestSampleSimplifies(t *testing.T) {
	cablesBody, _ := os.ReadFile("testdata/cables.json")
	landingsBody, _ := os.ReadFile("testdata/landings.json")
	source, err := New(config.Source{ID: "c", Type: "geography.cables", CacheDirectory: t.TempDir()}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.cache = pollutil.FileCache{Path: filepath.Join(t.TempDir(), "simplified.json")}
	source.fetchCables = func(context.Context) ([]byte, error) { return cablesBody, nil }
	source.fetchLanding = func(context.Context) ([]byte, error) { return landingsBody, nil }
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d", len(records))
	}
	var payload map[string]any
	_ = json.Unmarshal(records[0].Payload, &payload)
	if payload["kind"] != "cable" {
		t.Fatalf("%v", payload)
	}
}
