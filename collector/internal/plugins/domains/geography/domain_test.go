package geography

import (
	"context"
	"encoding/json"
	"strings"
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
	if messages[0].Properties["display.title"] != "Demo" {
		t.Fatalf("title %#v", messages[0].Properties["display.title"])
	}
	if messages[0].Properties["display.secondary"] != "submarine cable" {
		t.Fatalf("no landings means generic secondary, got %#v", messages[0].Properties["display.secondary"])
	}
	named, _ := json.Marshal(map[string]any{
		"kind": "cable", "cableId": "a", "name": "Demo", "color": "#32499f",
		"segments": [][][]float64{{{-5, 36}, {5, 37}}},
		"landings": []string{"Cadiz", "Algiers"},
	})
	messages, err = New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "cables", SourceInstanceID: "t", OriginalID: "c2", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: named,
	})
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].Properties["display.secondary"] != "Cadiz  //  Algiers" {
		t.Fatalf("landing names %#v", messages[0].Properties["display.secondary"])
	}
	for _, key := range []string{"display.primary", "display.secondary", "display.title"} {
		text, _ := messages[0].Properties[key].(string)
		lower := strings.ToLower(text)
		if strings.Contains(lower, "util") || strings.Contains(lower, "capacit") || strings.Contains(lower, "load") {
			t.Fatalf("invented load field %s=%q", key, text)
		}
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

func TestCableLandingSecondaryCapsAtEight(t *testing.T) {
	names := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"}
	cable, _ := json.Marshal(map[string]any{
		"kind": "cable", "cableId": "long", "name": "Many Landings", "color": "#32499f",
		"segments": [][][]float64{{{-5, 36}, {5, 37}}},
		"landings": names,
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "cables", SourceInstanceID: "t", OriginalID: "many", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: cable,
	})
	if err != nil {
		t.Fatal(err)
	}
	if messages[0].Properties["display.secondary"] != "A  //  B  //  C  //  D  //  E  //  F  //  G  //  H  //  +2" {
		t.Fatalf("cap %#v", messages[0].Properties["display.secondary"])
	}
}
