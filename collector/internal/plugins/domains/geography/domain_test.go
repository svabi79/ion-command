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
	if messages[0].Properties["visual.legendIndex"] != 0 || messages[0].Properties["visual.color"] != "0.78,0.58,0.26" {
		t.Fatalf("planned class expected, got %#v", messages[0].Properties)
	}
}

func TestCableLegendColors(t *testing.T) {
	cases := []struct {
		planned  bool
		lengthKm float64
		class    string
		color    string
		index    int
	}{
		{true, 100, "planned", "0.78,0.58,0.26", 0},
		{false, 100, "in service · short", "0.30,0.56,0.50", 1},
		{false, 1000, "in service · regional", "0.34,0.60,0.70", 2},
		{false, 5000, "in service · ocean", "0.30,0.42,0.66", 3},
		{false, 15000, "in service · trunk", "0.66,0.34,0.52", 4},
	}
	for _, tc := range cases {
		class, color := cableLegend(tc.planned, tc.lengthKm)
		if class != tc.class || color != tc.color {
			t.Fatalf("legend(%v, %.0f) = %q %q, want %q %q", tc.planned, tc.lengthKm, class, color, tc.class, tc.color)
		}
		if got := cableLegendIndex(tc.planned, tc.lengthKm); got != tc.index {
			t.Fatalf("index(%v, %.0f) = %d, want %d", tc.planned, tc.lengthKm, got, tc.index)
		}
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

func TestBorderCityRiverLandmark(t *testing.T) {
	domain := New()
	border, _ := json.Marshal(map[string]any{
		"kind": "border", "borderId": "fr-de", "name": "France / Germany",
		"segments": [][][]float64{{{6.0, 49.0}, {8.0, 48.0}}},
	})
	messages, err := domain.Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "naturalearth", SourceInstanceID: "t", OriginalID: "b", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: border,
	})
	if err != nil || len(messages) != 1 || messages[0].SemanticType != "geography.border" {
		t.Fatalf("border %v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
	if messages[0].Geometry.Type != "LineString" || messages[0].Properties["visual.legendIndex"] != 0 {
		t.Fatalf("border geometry/legend %#v", messages[0])
	}

	city, _ := json.Marshal(map[string]any{
		"kind": "city", "placeId": "tokyo", "name": "Tokyo", "latitude": 35.68, "longitude": 139.75,
		"population": 37732000, "capital": true, "lod": 0,
	})
	messages, err = domain.Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "naturalearth", SourceInstanceID: "t", OriginalID: "c", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: city,
	})
	if err != nil || messages[0].SemanticType != "geography.city" || messages[0].Geometry.Type != "Point" {
		t.Fatalf("city %v %#v", err, messages)
	}
	if messages[0].Properties["visual.lod"] != 0 || messages[0].Properties["display.title"] != "Tokyo" {
		t.Fatalf("city props %#v", messages[0].Properties)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}

	river, _ := json.Marshal(map[string]any{
		"kind": "river", "riverId": "nile", "name": "Nile", "lod": 0,
		"segments": [][][]float64{{{32.0, 15.0}, {31.0, 30.0}}},
		"labelLon": 32.0, "labelLat": 22.0,
	})
	messages, err = domain.Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "naturalearth", SourceInstanceID: "t", OriginalID: "r", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: river,
	})
	if err != nil || len(messages) != 2 {
		t.Fatalf("river %v %#v", err, messages)
	}
	if messages[0].SemanticType != "geography.river" || messages[0].Geometry.Type != "LineString" {
		t.Fatalf("river path %#v", messages[0])
	}
	if messages[1].SemanticType != "geography.landmark" || messages[1].Geometry.Type != "Point" {
		t.Fatalf("river label %#v", messages[1])
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
	if err := messages[1].Validate(); err != nil {
		t.Fatal(err)
	}

	peak, _ := json.Marshal(map[string]any{
		"kind": "peak", "placeId": "everest", "name": "Mount Everest",
		"latitude": 27.99, "longitude": 86.92, "elevation": 8848, "lod": 0,
	})
	messages, err = domain.Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "naturalearth", SourceInstanceID: "t", OriginalID: "p", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: peak,
	})
	if err != nil || messages[0].SemanticType != "geography.landmark" {
		t.Fatalf("peak %v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}

	country, _ := json.Marshal(map[string]any{
		"kind": "country", "placeId": "country-DEU", "name": "Germany",
		"latitude": 51.1, "longitude": 10.4, "lod": 0,
	})
	messages, err = domain.Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "naturalearth", SourceInstanceID: "t", OriginalID: "g", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: country,
	})
	if err != nil || messages[0].SemanticType != "geography.country" {
		t.Fatalf("country %v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
}
