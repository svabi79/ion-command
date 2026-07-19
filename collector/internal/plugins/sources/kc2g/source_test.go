package kc2g

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

// Trimmed from a live https://prop.kc2g.com/api/stations.json capture on
// 2026-07-19: one fresh reading, one months-stale station, one station whose
// autoscaler produced nulls.
const realFixture = `[
{"time": "2026-03-19T22:10:05", "source": "giro", "fof2": 8.6, "tec": 15.837, "foe": 3.32, "station": {"code": "AU930", "longitude": "262.3", "id": 1, "name": "Austin, TX, USA", "latitude": "30.4"}, "yf1": null, "scalef2": 38.14, "fbes": null, "fof1": null, "hme": 106.1, "id": 22008538, "md": "3.352", "hf2": null, "he": 90.0, "mufd": 28.827, "foes": 3.6, "yf2": 63.001, "cs": 100.0, "hmf2": 241.8, "hmf1": null},
{"time": "2026-07-19T08:25:01", "source": "giro", "fof2": 6.3, "tec": 19.634, "foe": 2.95, "station": {"code": "EA036", "longitude": "353.3", "id": 2, "name": "El Arenosillo, Spain", "latitude": "37.1"}, "yf1": null, "scalef2": 53.156, "fbes": null, "fof1": null, "hme": 98.871, "id": 22775893, "md": "3.342", "hf2": 389.751, "he": 98.824, "mufd": 21.055, "foes": 5.4, "yf2": 97.506, "cs": 80.0, "hmf2": 247.305, "hmf1": null},
{"time": "2026-07-19T08:25:01", "source": "giro", "fof2": null, "tec": null, "foe": null, "station": {"code": "EB040", "longitude": "0.5", "id": 3, "name": "Roquetes, Spain", "latitude": "40.8"}, "yf1": null, "scalef2": null, "fbes": null, "fof1": null, "hme": null, "id": 22775894, "md": null, "hf2": null, "he": null, "mufd": null, "foes": null, "cs": 0.0, "hmf2": null, "hmf1": null}
]`

func testSource(t *testing.T, body string) *Source {
	t.Helper()
	source, err := New(config.Source{ID: "test", Type: "ionosonde.kc2g"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context) ([]byte, error) { return []byte(body), nil }
	source.now = func() time.Time { return time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC) }
	return source
}

func TestSampleFiltersAndConverts(t *testing.T) {
	source := testSource(t, realFixture)
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected only the fresh EA036 reading, got %d records", len(records))
	}
	record := records[0]
	if record.Domain != "ionosphere" || record.SourcePluginID != "kc2g" {
		t.Fatalf("unexpected routing: %+v", record)
	}
	payload := string(record.Payload)
	for _, expected := range []string{`"stationId":"EA036"`, `"foF2Mhz":6.3`, `"mufdMhz":21.055`, `"m3000":3.342`, `"confidence":80`} {
		if !strings.Contains(payload, expected) {
			t.Fatalf("payload missing %s: %s", expected, payload)
		}
	}
	var decoded map[string]any
	if err := json.Unmarshal(record.Payload, &decoded); err != nil {
		t.Fatal(err)
	}
	// Longitudes arrive 0-360 and must be normalized to +-180.
	if lon := decoded["longitude"].(float64); lon < -6.75 || lon > -6.65 {
		t.Fatalf("longitude 353.3 should normalize to about -6.7, got %v", lon)
	}
}

func TestSampleDedupesAcrossPolls(t *testing.T) {
	source := testSource(t, realFixture)
	first, err := source.sample(context.Background())
	if err != nil || len(first) != 1 {
		t.Fatalf("first sample: %v (%d records)", err, len(first))
	}
	second, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("unchanged readings must not be re-emitted, got %d", len(second))
	}
}

func TestPollFloorRejected(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "ionosonde.kc2g", PollSeconds: 60}, slog.Default()); err == nil {
		t.Fatal("expected sub-five-minute poll interval to be rejected")
	}
}
