package wildfire

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestDetectionNormalizesViirsCategoryConfidence(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"detectionId": "N-2026-08-27-1000-41.93654--117.36444", "longitude": -117.36444, "latitude": 41.93654,
		"instrument": "VIIRS", "satelliteCode": "N", "satelliteLabel": "Suomi NPP",
		"confidenceRaw": "nominal", "brightnessK": 338.5, "brightness2K": 291.75, "frpMw": 8.71, "dayNight": "N",
	})
	observed := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	record := plugins.RawRecord{SourcePluginID: "firms", SourceInstanceID: "test", OriginalID: "firms-test-one", Domain: "wildfire", ObservedUTC: observed, Payload: payload}

	messages, err := New().Normalize(context.Background(), record)
	if err != nil || len(messages) != 1 {
		t.Fatalf("normalize failed: %v (%d)", err, len(messages))
	}
	event := messages[0]
	if event.SemanticType != "wildfire.detection" {
		t.Fatalf("unexpected semantic type: %q", event.SemanticType)
	}
	if event.EntityID != "wildfire:detection:N-2026-08-27-1000-41.93654--117.36444" {
		t.Fatalf("unexpected entity id: %q", event.EntityID)
	}
	if event.Time.ValidUntilUTC == nil || event.Time.ValidUntilUTC.Sub(observed) != 6*time.Hour {
		t.Fatalf("detections must carry a six-hour validity window: %v", event.Time.ValidUntilUTC)
	}
	if event.Quality.Measured == nil || !*event.Quality.Measured {
		t.Fatal("the sensor reading itself is measured, not simulated")
	}
	if event.Quality.Confidence == nil || math.Abs(*event.Quality.Confidence-0.6) > 1e-9 {
		t.Fatalf("VIIRS 'nominal' must map to the documented ordinal 0.6, got %v", event.Quality.Confidence)
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("invalid canonical event: %v", err)
	}

	// The task's explicit hard requirement: never present a detection as a
	// confirmed fire. Check both the machine-checkable flag and the text a
	// tooltip would actually show.
	if confirmed, _ := event.Properties["confirmed"].(bool); confirmed {
		t.Fatal(`properties["confirmed"] must be false`)
	}
	if event.Properties["detectionType"] != "thermal_anomaly" {
		t.Fatalf("unexpected detectionType: %v", event.Properties["detectionType"])
	}
	secondary, _ := event.Properties["display.secondary"].(string)
	if !strings.Contains(secondary, "not a confirmed fire") {
		t.Fatalf("display.secondary must say this is not a confirmed fire, got %q", secondary)
	}
	primary, _ := event.Properties["display.primary"].(string)
	if !strings.Contains(primary, "Suomi NPP") || !strings.Contains(primary, "nominal") {
		t.Fatalf("display.primary must carry satellite and confidence for the tooltip, got %q", primary)
	}
	if event.Properties["confidenceRaw"] != "nominal" {
		t.Fatalf("raw confidence must be carried through verbatim for provenance, got %v", event.Properties["confidenceRaw"])
	}
	if event.Properties["visual.icon"] != "wildfire" {
		t.Fatalf("unexpected visual.icon: %v", event.Properties["visual.icon"])
	}
}

func TestDetectionNormalizesModisPercentConfidence(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"detectionId": "T-2026-08-27-0357-35.90752--121.39445", "longitude": -121.39445, "latitude": 35.90752,
		"instrument": "MODIS", "satelliteCode": "T", "satelliteLabel": "Terra",
		"confidenceRaw": "73", "brightnessK": 322.08, "brightness2K": 298.04, "frpMw": 42.34, "dayNight": "N",
	})
	record := plugins.RawRecord{SourcePluginID: "firms", SourceInstanceID: "test", OriginalID: "firms-test-two", Domain: "wildfire", ObservedUTC: time.Now().UTC(), Payload: payload}

	messages, err := New().Normalize(context.Background(), record)
	if err != nil || len(messages) != 1 {
		t.Fatalf("normalize failed: %v (%d)", err, len(messages))
	}
	event := messages[0]
	if event.Quality.Confidence == nil || math.Abs(*event.Quality.Confidence-0.73) > 1e-9 {
		t.Fatalf("MODIS numeric confidence must divide by 100 directly, got %v", event.Quality.Confidence)
	}
	primary, _ := event.Properties["display.primary"].(string)
	if !strings.Contains(primary, "73%") {
		t.Fatalf("display.primary should render numeric confidence as a percent, got %q", primary)
	}
}

func TestNormalizeRequiresID(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"longitude": 1.0, "latitude": 2.0})
	record := plugins.RawRecord{Payload: payload, ObservedUTC: time.Now().UTC()}
	if _, err := New().Normalize(context.Background(), record); err == nil {
		t.Fatal("a detection without an id must be rejected")
	}
}

func TestNormalizedConfidenceMapping(t *testing.T) {
	cases := []struct {
		raw  string
		want *float64
	}{
		{"low", ptr(0.3)},
		{"nominal", ptr(0.6)},
		{"high", ptr(0.9)},
		{"LOW", ptr(0.3)}, // FIRMS lower-cases these, but be case-insensitive anyway.
		{"0", ptr(0.0)},
		{"100", ptr(1.0)},
		{"73", ptr(0.73)},
		{"", nil},
		{"garbage", nil},
	}
	for _, c := range cases {
		got := normalizedConfidence(c.raw)
		if (got == nil) != (c.want == nil) {
			t.Errorf("normalizedConfidence(%q) = %v, want %v", c.raw, got, c.want)
			continue
		}
		if got != nil && math.Abs(*got-*c.want) > 1e-9 {
			t.Errorf("normalizedConfidence(%q) = %v, want %v", c.raw, *got, *c.want)
		}
	}
}

func TestMarkerScaleMonotonicAndBounded(t *testing.T) {
	prev := markerScale(0)
	if prev < 0.8 {
		t.Fatalf("marker scale floor violated at FRP=0: %v", prev)
	}
	for _, frp := range []float64{1, 5, 20, 100, 500, 1e6} {
		scale := markerScale(frp)
		if scale < prev {
			t.Fatalf("marker scale must not shrink as FRP grows: FRP=%v scale=%v < previous %v", frp, scale, prev)
		}
		if scale > 3.5 {
			t.Fatalf("marker scale ceiling violated at FRP=%v: %v", frp, scale)
		}
		prev = scale
	}
}

func ptr(v float64) *float64 { return &v }
