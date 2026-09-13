package geography

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestRegionAndCable(t *testing.T) {
	region, _ := json.Marshal(map[string]any{"kind": "region", "name": "Alps", "regionKind": "region", "latitude": 46.5, "longitude": 9.0})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "naturalearth", SourceInstanceID: "t", OriginalID: "r", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: region,
	})
	if err != nil || messages[0].SemanticType != "geography.region" {
		t.Fatalf("%v %#v", err, messages)
	}
	cable, _ := json.Marshal(map[string]any{
		"kind": "cable", "cableId": "a", "name": "Demo", "color": "#32499f",
		"segments": [][][]float64{{{-5, 36}, {0, 42}, {5, 37}}, {{6, 37}, {8, 38}}},
	})
	messages, err = New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "cables", SourceInstanceID: "t", OriginalID: "c", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: cable,
	})
	if err != nil || messages[0].Geometry.Type != "MultiLineString" {
		t.Fatalf("%v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
	if messages[0].Properties["visual.legendIndex"] != 2 {
		t.Fatalf("regional class expected, got %#v", messages[0].Properties)
	}
	planned, _ := json.Marshal(map[string]any{
		"kind": "cable", "cableId": "b", "name": "Soon", "color": "#939597",
		"segments": [][][]float64{{{-8, 35}, {-6, 36}}},
	})
	messages, err = New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "cables", SourceInstanceID: "t", OriginalID: "p", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: planned,
	})
	if err != nil || messages[0].Geometry.Type != "LineString" {
		t.Fatalf("%v %#v", err, messages)
	}
	if messages[0].Properties["visual.legendIndex"] != 0 || messages[0].Properties["visual.color"] != "1.00,0.72,0.18" {
		t.Fatalf("planned class expected, got %#v", messages[0].Properties)
	}
}
