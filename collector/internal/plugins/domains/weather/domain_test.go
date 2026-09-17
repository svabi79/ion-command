package weather

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestLightningNormalizesAsGenericMeasuredPoint(t *testing.T) {
	payload, err := json.Marshal(rawLightning{StrikeID: "strike-1", Longitude: 8.3, Latitude: 47.2, PeakCurrentKa: -21.4})
	if err != nil {
		t.Fatal(err)
	}
	record := plugins.RawRecord{SourcePluginID: "mock.lightning", SourceInstanceID: "weather-a", OriginalID: "strike-1", Domain: "weather", ObservedUTC: time.Now().UTC(), Payload: payload}
	messages, err := New().Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one observation, got %d", len(messages))
	}
	message := messages[0]
	if message.MessageType != events.MessageObservation || message.SemanticType != "weather.lightning" || message.Geometry.Type != "Point" {
		t.Fatalf("unexpected canonical lightning message: %#v", message)
	}
	if message.Quality.Measured == nil || !*message.Quality.Measured {
		t.Fatal("a detected lightning strike must be classified as measured")
	}
	if err := message.Validate(); err != nil {
		t.Fatalf("generic canonical point rejected: %v", err)
	}
}

func TestStormNormalizes(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"kind": "storm", "stormId": "ep142026", "name": "Norbert", "classLabel": "Tropical Storm",
		"intensityKt": 55.0, "pressureHpa": 996.0, "latitude": 17.2, "longitude": -126.6, "provider": "nhc",
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "nhc", SourceInstanceID: "t", OriginalID: "ep142026", Domain: "weather",
		ObservedUTC: time.Now().UTC(), Payload: payload,
	})
	if err != nil || len(messages) != 1 || messages[0].SemanticType != "weather.storm" {
		t.Fatalf("%v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStormConeNormalizesAsArea(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"kind": "storm-cone", "stormId": "ep142026", "name": "Norbert", "classLabel": "Tropical Storm",
		"advisory": "014", "coneKind": "track", "provider": "nhc", "product": "5-day forecast cone",
		"rings": [][][]float64{{{-130.5, 17.9}, {-129.0, 18.0}, {-128.5, 20.0}, {-131.0, 19.5}, {-130.5, 17.9}}},
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "nhc", SourceInstanceID: "t", OriginalID: "ep142026:cone", Domain: "weather",
		ObservedUTC: time.Now().UTC(), Payload: payload,
	})
	if err != nil || len(messages) != 1 {
		t.Fatalf("%v %#v", err, messages)
	}
	message := messages[0]
	if message.MessageType != events.MessageArea || message.SemanticType != "weather.storm.cone" || message.Geometry.Type != "Polygon" {
		t.Fatalf("unexpected cone message: %#v", message)
	}
	if message.EntityID != "weather:storm:ep142026" {
		t.Fatalf("entity %s", message.EntityID)
	}
	if len(message.Relationships) != 1 || message.Relationships[0].TargetID != "weather:storm:ep142026" {
		t.Fatalf("relationships %#v", message.Relationships)
	}
	if err := message.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDroughtFloodBuoy(t *testing.T) {
	ring := [][][]float64{{{-100, 35}, {-98, 35}, {-98, 37}, {-100, 37}, {-100, 35}}}
	drought, _ := json.Marshal(map[string]any{
		"kind": "drought", "areaId": "D2", "label": "Severe Drought", "class": "D2", "rings": ring,
		"attribution": "USDM",
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "usdm", SourceInstanceID: "t", OriginalID: "d", Domain: "weather",
		ObservedUTC: time.Now().UTC(), Payload: drought,
	})
	if err != nil || messages[0].SemanticType != "weather.drought" || messages[0].Geometry.Type != "Polygon" {
		t.Fatalf("drought %v %#v", err, messages)
	}
	flood, _ := json.Marshal(map[string]any{
		"kind": "flood", "areaId": "rfm-1", "title": "GloFAS RFM", "label": "rapid flood mapping", "rings": ring,
	})
	messages, err = New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "glofas", SourceInstanceID: "t", OriginalID: "f", Domain: "weather",
		ObservedUTC: time.Now().UTC(), Payload: flood,
	})
	if err != nil || messages[0].SemanticType != "weather.flood" {
		t.Fatalf("flood %v %#v", err, messages)
	}
	buoy, _ := json.Marshal(map[string]any{
		"kind": "buoy", "stationId": "41001", "buoyKind": "buoy", "windMs": 6.2, "waveM": 1.5,
		"latitude": 34.7, "longitude": -72.7, "attribution": "NDBC",
	})
	messages, err = New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "ndbc", SourceInstanceID: "t", OriginalID: "b", Domain: "weather",
		ObservedUTC: time.Now().UTC(), Payload: buoy,
	})
	if err != nil || messages[0].SemanticType != "weather.buoy" || messages[0].Geometry.Type != "Point" {
		t.Fatalf("buoy %v %#v", err, messages)
	}
}
