package swpc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func testSource(t *testing.T, responses map[string]string) *Source {
	t.Helper()
	source, err := New(config.Source{ID: "test", Type: "spaceweather.swpc"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context, url string) ([]byte, error) {
		for key, body := range responses {
			if strings.Contains(url, key) {
				return []byte(body), nil
			}
		}
		return nil, fmt.Errorf("no stub for %s", url)
	}
	return source
}

func TestSampleParsesProducts(t *testing.T) {
	source := testSource(t, map[string]string{
		"noaa-planetary-k-index": `[{"time_tag":"2026-07-18T21:00:00","Kp":3.0},{"time_tag":"2026-07-19T00:00:00","Kp":2.33}]`,
		"10cm-flux":              `[{"flux":165,"time_tag":"2026-07-18T20:00:00"}]`,
		"solar-wind-speed":       `[{"proton_speed":434.4,"time_tag":"2026-07-19T05:41:00Z"}]`,
		"solar-wind-mag-field":   `[{"bt":5.1,"bz_gsm":-2.3,"time_tag":"2026-07-19T05:41:00Z"}]`,
		"xrays-6-hour": `[{"time_tag":"2026-07-19T14:57:00Z","flux":9.1e-09,"energy":"0.05-0.4nm"},{"time_tag":"2026-07-19T14:58:00Z","flux":7.818933909220505e-07,"energy":"0.1-0.8nm"},{"time_tag":"2026-07-19T14:59:00Z","flux":9.1e-09,"energy":"0.05-0.4nm"}]`,
		"wwv": ":Product: Geophysical Alert Message wwv.txt\n:Issued: 2026 Jul 19 0605 UTC\nSolar-terrestrial indices for 18 July follow.\nSolar flux 110 and estimated planetary A-index 4.\nThe estimated planetary K-index at 0600 UTC on 19 July was 0.67.\n",
	})
	record, err := source.sample(context.Background())
	if err != nil {
		t.Fatalf("sample failed: %v", err)
	}
	if record.Domain != "spaceweather" || record.SourcePluginID != "swpc" {
		t.Fatalf("unexpected routing: %+v", record)
	}
	payload := string(record.Payload)
	for _, expected := range []string{`"kp":2.33`, `"solarFlux":165`, `"solarWindSpeedKms":434.4`, `"imfBzNt":-2.3`, `"aIndex":4`, `"xrayClass":"B7.8"`} {
		if !strings.Contains(payload, expected) {
			t.Fatalf("payload missing %s: %s", expected, payload)
		}
	}
}

func TestSampleDegradesWhenSummariesFail(t *testing.T) {
	source := testSource(t, map[string]string{
		"noaa-planetary-k-index": `[{"time_tag":"2026-07-19T03:00:00","Kp":"4.67"}]`,
	})
	record, err := source.sample(context.Background())
	if err != nil {
		t.Fatalf("sample failed: %v", err)
	}
	payload := string(record.Payload)
	if !strings.Contains(payload, `"kp":4.67`) || !strings.Contains(payload, `"solarFlux":null`) || !strings.Contains(payload, `"aIndex":null`) {
		t.Fatalf("unexpected degraded payload: %s", payload)
	}
}

func TestSampleFailsWithoutKp(t *testing.T) {
	source := testSource(t, map[string]string{})
	if _, err := source.sample(context.Background()); err == nil {
		t.Fatal("expected failure when the Kp product is unreachable")
	}
}
