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

func TestSampleKeepsRouteAndLandings(t *testing.T) {
	cablesBody, _ := os.ReadFile("testdata/cables.json")
	landingsBody, _ := os.ReadFile("testdata/landings.json")
	source, err := New(config.Source{ID: "c", Type: "geography.cables", CacheDirectory: t.TempDir()}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.cache = pollutil.FileCache{Path: filepath.Join(t.TempDir(), "routes.json")}
	source.fetchCables = func(context.Context) ([]byte, error) { return cablesBody, nil }
	source.fetchLanding = func(context.Context) ([]byte, error) { return landingsBody, nil }
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records", len(records))
	}
	var demo map[string]any
	if err := json.Unmarshal(records[0].Payload, &demo); err != nil {
		t.Fatal(err)
	}
	if demo["kind"] != "cable" || demo["cableId"] != "cable-a" {
		t.Fatalf("%v", demo)
	}
	segments, ok := demo["segments"].([]any)
	if !ok || len(segments) != 2 {
		t.Fatalf("expected two route segments, got %#v", demo["segments"])
	}
	first, ok := segments[0].([]any)
	if !ok || len(first) != 3 {
		t.Fatalf("dogleg should keep the mid vertex, got %#v", segments[0])
	}
	mid, ok := first[1].([]any)
	if !ok || mid[1].(float64) != 42 {
		t.Fatalf("mid vertex lost: %#v", first)
	}
	var planned map[string]any
	if err := json.Unmarshal(records[1].Payload, &planned); err != nil {
		t.Fatal(err)
	}
	if planned["color"] != "#939597" {
		t.Fatalf("planned color %v", planned["color"])
	}
	var landing map[string]any
	if err := json.Unmarshal(records[2].Payload, &landing); err != nil {
		t.Fatal(err)
	}
	if landing["kind"] != "landing" || landing["landingId"] != "lp-1" {
		t.Fatalf("%v", landing)
	}
}

func TestDecimateKeepsEndsAndDropsNearDuplicates(t *testing.T) {
	line := [][]float64{{-5, 36}, {-4.99, 36.01}, {5, 37}}
	got := decimateLine(line)
	if len(got) != 2 {
		t.Fatalf("near-duplicate should drop, got %#v", got)
	}
	if got[0][0] != -5 || got[1][0] != 5 {
		t.Fatalf("ends must stay %#v", got)
	}
}
