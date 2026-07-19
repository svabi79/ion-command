package geophysics

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestQuakeNormalizes(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"quakeId": "aka2026oesaet", "longitude": -149.853, "latitude": 61.743,
		"magnitude": 4.7, "depthKm": 31.5, "place": "9 km E of Willow, Alaska",
	})
	record := plugins.RawRecord{SourcePluginID: "usgs", SourceInstanceID: "test", OriginalID: "one", Domain: "geophysics", ObservedUTC: time.Now().UTC(), Payload: payload}
	messages, err := New().Normalize(context.Background(), record)
	if err != nil || len(messages) != 1 {
		t.Fatalf("normalize failed: %v (%d)", err, len(messages))
	}
	event := messages[0]
	if event.SemanticType != "geophysics.earthquake" || event.EntityID != "geophysics:quake:aka2026oesaet" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.Time.ValidUntilUTC == nil || event.Time.ValidUntilUTC.Sub(record.ObservedUTC) != 2*time.Hour {
		t.Fatalf("quakes must carry a two-hour validity: %v", event.Time.ValidUntilUTC)
	}
	scale := event.Properties["visual.markerScale"].(float64)
	if scale < 2.4 || scale > 2.5 {
		t.Fatalf("magnitude 4.7 should scale to ~2.45, got %v", scale)
	}
	if event.Properties["display.title"] != "M 4.7 Earthquake" {
		t.Fatalf("unexpected title: %v", event.Properties["display.title"])
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("invalid canonical event: %v", err)
	}
}
