package solar

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
	solarmat "github.com/ion-command/ion-command/collector/internal/solar"
)

func TestGraylineNormalizesToArea(t *testing.T) {
	ring := solarmat.TwilightBand(10, -30)
	payload, _ := json.Marshal(map[string]any{
		"kind": "grayline", "rings": [][][]float64{ring},
		"subsolarLatitude": 10.0, "subsolarLongitude": -30.0,
		"bandInnerDeg": 90.0, "bandOuterDeg": 102.0,
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "grayline", SourceInstanceID: "test", OriginalID: "gray-1",
		Domain: "solar", ObservedUTC: time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
		Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].SemanticType != "solar.grayline" || messages[0].Geometry.Type != "Polygon" {
		t.Fatalf("unexpected envelope: %#v", messages)
	}
	if messages[0].Properties["visual.opacity"] != 0.12 {
		t.Fatalf("grayline must stay restrained, got opacity %v", messages[0].Properties["visual.opacity"])
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatalf("invalid grayline envelope: %v", err)
	}
}
