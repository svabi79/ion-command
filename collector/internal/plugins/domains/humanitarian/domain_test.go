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
