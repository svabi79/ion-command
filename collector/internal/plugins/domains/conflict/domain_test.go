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
		"eventId": "ged-1", "title": "Ogaden clash", "eventKind": "state-based",
		"country": "Ethiopia", "deaths": 3, "latitude": 8.2, "longitude": 43.5,
		"attribution": "UCDP GED",
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "ucdp", SourceInstanceID: "t", OriginalID: "ged-1", Domain: "conflict",
		ObservedUTC: time.Now().UTC(), Payload: payload,
	})
	if err != nil || len(messages) != 1 || messages[0].SemanticType != "conflict.event" {
		t.Fatalf("%v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
}
