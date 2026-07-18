package hfpropagation

import (
	"context"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
)

func TestContextCombinesTwoDomains(t *testing.T) {
	processor := New()
	source := events.SourceRef{PluginID: "test", InstanceID: "test"}
	spaceWeather := events.NewEnvelope("sw", "spaceweather", "spaceweather.state", events.MessageObservation, source, time.Now())
	spaceWeather.Properties["kp"] = 3.3
	if _, err := processor.Process(context.Background(), spaceWeather); err != nil {
		t.Fatal(err)
	}
	radio := events.NewEnvelope("link", "hamradio", "radio.reception", events.MessageRelationship, source, time.Now())
	derived, err := processor.Process(context.Background(), radio)
	if err != nil {
		t.Fatal(err)
	}
	if len(derived) != 1 || derived[0].TargetID != "link" {
		t.Fatalf("unexpected context output: %#v", derived)
	}
	if derived[0].Quality.Confidence == nil || *derived[0].Quality.Confidence >= 0.5 {
		t.Fatal("bootstrap context must be explicit low confidence")
	}
}
