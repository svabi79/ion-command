package hamradio

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestSpotNormalizesThroughCanonicalModel(t *testing.T) {
	payload, _ := json.Marshal(rawSpot{SpotID: "one", TXCallsign: "N0CALL", RXCallsign: "TEST1", TXLongitude: 8, TXLatitude: 47, RXLongitude: -74, RXLatitude: 41, FrequencyHz: 14074000, Band: "20m", Mode: "FT8", SNRDb: -11})
	record := plugins.RawRecord{SourcePluginID: "mock.radio", SourceInstanceID: "test", OriginalID: "one", Domain: "hamradio", ObservedUTC: time.Now(), Payload: payload}
	domain := New()
	messages, err := domain.Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("wanted two entities and one relationship, got %d", len(messages))
	}
	link := messages[2]
	if link.SemanticType != "radio.reception" || link.Geometry.Type != "GreatCircle" {
		t.Fatalf("unexpected link: %#v", link)
	}
	if link.Properties["display.title"] != "Observed Link" || link.Properties["display.from"] != "N0CALL" || link.Properties["display.to"] != "TEST1" {
		t.Fatalf("missing domain-supplied generic display metadata: %#v", link.Properties)
	}
	if err := link.Validate(); err != nil {
		t.Fatalf("invalid canonical link: %v", err)
	}
	repeated, err := domain.Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated) != 1 || repeated[0].SemanticType != "radio.reception" {
		t.Fatalf("repeat should only emit the relationship, got %#v", repeated)
	}
}
