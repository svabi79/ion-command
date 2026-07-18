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
