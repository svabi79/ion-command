package ionosphere

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestSoundingNormalizes(t *testing.T) {
	hm := 247.305
	payload, _ := json.Marshal(map[string]any{
		"stationId": "EA036", "name": "El Arenosillo, Spain",
		"latitude": 37.1, "longitude": -6.7,
		"foF2Mhz": 6.3, "mufdMhz": 21.055, "hmF2Km": hm, "m3000": 3.342, "confidence": 80.0,
	})
	record := plugins.RawRecord{SourcePluginID: "kc2g", SourceInstanceID: "test", OriginalID: "one", Domain: "ionosphere", ObservedUTC: time.Now(), Payload: payload}
	messages, err := New().Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one observation, got %d", len(messages))
	}
	event := messages[0]
	if event.SemanticType != "ionosphere.sounding" || event.EntityID != "ionosphere:station:EA036" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.Geometry.Type != "Point" {
		t.Fatalf("expected point geometry: %#v", event.Geometry)
	}
	if event.Properties["mufdMhz"] != 21.055 || event.Properties["display.secondary"] != "El Arenosillo, Spain" {
		t.Fatalf("unexpected properties: %#v", event.Properties)
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("invalid canonical event: %v", err)
	}
}

func TestSoundingRequiresCoreValues(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"stationId": "X", "latitude": 1.0, "longitude": 2.0, "foF2Mhz": 0.0, "mufdMhz": 0.0})
	record := plugins.RawRecord{SourcePluginID: "kc2g", SourceInstanceID: "test", OriginalID: "two", Domain: "ionosphere", ObservedUTC: time.Now(), Payload: payload}
	if _, err := New().Normalize(context.Background(), record); err == nil {
		t.Fatal("expected soundings without foF2/MUF to be rejected")
	}
}
