package conflict

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestEventNormalizes(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"kind": "conflict", "eventId": "IRQ1234", "eventType": "Battles",
		"subEventType": "Armed clash", "country": "Iraq", "location": "Baghdad",
		"latitude": 33.3152, "longitude": 44.3661, "fatalities": 3,
		"attribution": "Armed Conflict Location & Event Data Project (ACLED); www.acleddata.com",
	})
	observed := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "acled", SourceInstanceID: "a", OriginalID: "acled-IRQ1234",
		Domain: "conflict", ObservedUTC: observed, Payload: payload,
	})
	if err != nil || len(messages) != 1 {
		t.Fatalf("%v %#v", err, messages)
	}
	event := messages[0]
	if event.SemanticType != "conflict.event" || event.EntityID != "conflict:event:IRQ1234" {
		t.Fatalf("%#v", event)
	}
	if event.Time.ValidUntilUTC == nil || event.Time.ValidUntilUTC.Sub(observed) != 24*time.Hour {
		t.Fatalf("validity %v", event.Time.ValidUntilUTC)
	}
	if event.Properties["display.title"] != "Baghdad, Iraq" {
		t.Fatalf("title %v", event.Properties["display.title"])
	}
	if event.Properties["visual.markerScale"].(float64) != 1.0 {
		t.Fatalf("must not invent a severity-scaled marker, got %v", event.Properties["visual.markerScale"])
	}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
}
