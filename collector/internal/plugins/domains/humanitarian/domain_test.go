package humanitarian

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestDisplacementNormalizes(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"kind": "displacement", "country": "TUR", "name": "Türkiye", "role": "host",
		"population": 2387842, "latitude": 39.0, "longitude": 35.2, "attribution": "UNHCR via HDX HAPI",
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "hapi", SourceInstanceID: "t", OriginalID: "x", Domain: "humanitarian",
		ObservedUTC: time.Now().UTC(), Payload: payload,
	})
	if err != nil || len(messages) != 1 || messages[0].SemanticType != "humanitarian.displacement" {
		t.Fatalf("%v %#v", err, messages)
	}
	if messages[0].EntityID != "humanitarian:host:TUR" {
		t.Fatalf("entity %s", messages[0].EntityID)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDisasterNormalizes(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"kind": "disaster", "disasterId": "50599", "title": "Afghanistan: Earthquake - Sep 2025",
		"status": "current", "glide": "EQ-2025-000123-AFG", "category": "Earthquake",
		"country": "Afghanistan", "latitude": 33.94, "longitude": 67.71,
		"attribution": "ReliefWeb / OCHA",
	})
	observed := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "reliefweb", SourceInstanceID: "r", OriginalID: "reliefweb-50599",
		Domain: "humanitarian", ObservedUTC: observed, Payload: payload,
	})
	if err != nil || len(messages) != 1 {
		t.Fatalf("%v %#v", err, messages)
	}
	event := messages[0]
	if event.SemanticType != "humanitarian.disaster" || event.EntityID != "humanitarian:disaster:50599" {
		t.Fatalf("%#v", event)
	}
	if event.Time.ValidUntilUTC == nil || event.Time.ValidUntilUTC.Sub(observed) != 24*time.Hour {
		t.Fatalf("validity %v", event.Time.ValidUntilUTC)
	}
	if event.Properties["display.title"] != "Afghanistan: Earthquake - Sep 2025" {
		t.Fatalf("title %v", event.Properties["display.title"])
	}
	if event.Properties["visual.markerScale"].(float64) != 1.3 {
		t.Fatalf("scale %v", event.Properties["visual.markerScale"])
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
}
