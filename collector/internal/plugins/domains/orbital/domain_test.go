package orbital

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestPositionEmitsFootprintOnCadence(t *testing.T) {
	payload, _ := json.Marshal(rawPosition{
		SatID: "25544", Name: "ISS (ZARYA)", Latitude: 10, Longitude: 20, AltKm: 420,
	})
	// Unix/10%3 == 0 → 30 s cadence hit.
	at := time.Unix(300, 0).UTC()
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "celestrak", SourceInstanceID: "test", OriginalID: "sat-1",
		Domain: "orbital", ObservedUTC: at, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected position + footprint, got %d", len(messages))
	}
	if messages[0].SemanticType != "orbital.position" {
		t.Fatalf("first %s", messages[0].SemanticType)
	}
	fp := messages[1]
	if fp.SemanticType != "orbital.footprint" || fp.Geometry.Type != "Polygon" {
		t.Fatalf("unexpected footprint: %#v", fp)
	}
	if fp.Properties["visual.opacity"] != 0.10 {
		t.Fatalf("footprint should stay restrained, opacity %v", fp.Properties["visual.opacity"])
	}
	if fp.Properties["visual.defaultHidden"] != true {
		t.Fatalf("footprints must default hidden, defaultHidden=%v", fp.Properties["visual.defaultHidden"])
	}
	if fp.Properties["visual.pinKey"] != "25544" {
		t.Fatalf("pinKey %v", fp.Properties["visual.pinKey"])
	}
	if err := fp.Validate(); err != nil {
		t.Fatalf("invalid footprint: %v", err)
	}
}

func TestPositionSkipsFootprintOffCadence(t *testing.T) {
	payload, _ := json.Marshal(rawPosition{
		SatID: "25544", Name: "ISS (ZARYA)", Latitude: 10, Longitude: 20, AltKm: 420,
	})
	at := time.Unix(310, 0).UTC() // 31 % 3 == 1
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "celestrak", SourceInstanceID: "test", OriginalID: "sat-2",
		Domain: "orbital", ObservedUTC: at, Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].SemanticType != "orbital.position" {
		t.Fatalf("off-cadence must be position only, got %#v", messages)
	}
}
