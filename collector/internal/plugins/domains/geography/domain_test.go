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
	cable, _ := json.Marshal(map[string]any{"kind": "cable", "cableId": "a", "name": "Demo", "fromLon": -5, "fromLat": 36, "toLon": 5, "toLat": 37})
	messages, err = New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "cables", SourceInstanceID: "t", OriginalID: "c", Domain: "geography",
		ObservedUTC: time.Now().UTC(), Payload: cable,
	})
	if err != nil || messages[0].Geometry.Type != "GreatCircle" {
		t.Fatalf("%v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
}
