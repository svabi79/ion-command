package nhc

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleKeepsNamedCentre(t *testing.T) {
	body, err := os.ReadFile("testdata/current.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "n", Type: "weather.nhc"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context, string) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["name"] != "Norbert" || payload["classLabel"] != "Tropical Storm" {
		t.Fatalf("%v", payload)
	}
	if payload["longitude"].(float64) != -126.6 {
		t.Fatalf("lon %v", payload["longitude"])
	}
}

func TestSampleEmitsConeFromKmz(t *testing.T) {
	current, err := os.ReadFile("testdata/current_with_cone.json")
	if err != nil {
		t.Fatal(err)
	}
	kmz, err := os.ReadFile("testdata/cone.kmz")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "n", Type: "weather.nhc"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context, rawURL string) ([]byte, error) {
		if strings.Contains(rawURL, "CONE") || strings.HasSuffix(rawURL, ".kmz") {
			return kmz, nil
		}
		return current, nil
	}
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
	var cone map[string]any
	if err := json.Unmarshal(records[1].Payload, &cone); err != nil {
		t.Fatal(err)
	}
	if cone["kind"] != "storm-cone" || cone["stormId"] != "ep142026" {
		t.Fatalf("%v", cone)
	}
	rings, ok := cone["rings"].([]any)
	if !ok || len(rings) != 1 {
		t.Fatalf("rings %v", cone["rings"])
	}
	ring, _ := rings[0].([]any)
	if len(ring) < 4 || len(ring) > maxRingVertices {
		t.Fatalf("ring length %d", len(ring))
	}
	first, _ := ring[0].([]any)
	if len(first) < 2 || first[0].(float64) > -120 {
		t.Fatalf("first vertex %v", first)
	}
}

func TestParseConeGeometryFromKmz(t *testing.T) {
	kmz, err := os.ReadFile("testdata/cone.kmz")
	if err != nil {
		t.Fatal(err)
	}
	rings, err := parseConeGeometry(kmz)
	if err != nil || len(rings) != 1 {
		t.Fatalf("%v %d", err, len(rings))
	}
	if len(rings[0]) < 4 || len(rings[0]) > maxRingVertices {
		t.Fatalf("vertices %d", len(rings[0]))
	}
	if !samePoint(rings[0][0], rings[0][len(rings[0])-1]) {
		t.Fatal("ring must be closed")
	}
}

func TestParseConeGeometryFromKML(t *testing.T) {
	kml := []byte(`<kml><Polygon><outerBoundaryIs><LinearRing><coordinates>
-80.0,25.0,0 -79.0,25.0,0 -79.0,26.0,0 -80.0,26.0,0 -80.0,25.0,0
</coordinates></LinearRing></outerBoundaryIs></Polygon></kml>`)
	rings, err := parseConeGeometry(kml)
	if err != nil || len(rings) != 1 || len(rings[0]) != 5 {
		t.Fatalf("%v %#v", err, rings)
	}
	if rings[0][0][0] != -80 || rings[0][2][1] != 26 {
		t.Fatalf("%v", rings[0])
	}
}

func TestEmptyStorms(t *testing.T) {
	source, err := New(config.Source{ID: "n", Type: "weather.nhc"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context, string) ([]byte, error) { return []byte(`{"activeStorms":[]}`), nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 0 {
		t.Fatalf("%v %d", err, len(records))
	}
}

func TestPollFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "weather.nhc", PollSeconds: 60}, slog.Default()); err == nil {
		t.Fatal("expected floor rejection")
	}
}

func TestDownsampleRingKeepsClosure(t *testing.T) {
	ring := make([][]float64, 0, 400)
	for i := 0; i < 399; i++ {
		ring = append(ring, []float64{float64(i%180) - 90, 10})
	}
	out := downsampleRing(ring, 20)
	if len(out) > 20 || len(out) < 4 {
		t.Fatalf("len %d", len(out))
	}
	if !samePoint(out[0], out[len(out)-1]) {
		t.Fatal("downsampled ring must close")
	}
}
