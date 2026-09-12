package space

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestLaunchNormalizes(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"launchId": "ll-1", "name": "Falcon 9 | Demo", "status": "Go",
		"net": "2026-09-13T12:00:00Z", "padName": "SLC-40", "latitude": 28.5, "longitude": -80.5,
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "launchlibrary", SourceInstanceID: "t", OriginalID: "x", Domain: "space",
		ObservedUTC: time.Now().UTC(), Payload: payload,
	})
	if err != nil || len(messages) != 1 || messages[0].SemanticType != "space.launch" {
		t.Fatalf("%v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPadNormalizes(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"kind": "pad", "padId": "12", "name": "Kennedy Space Center, USA", "country": "United States",
		"latitude": 28.6, "longitude": -80.6, "active": true, "launches": 200,
	})
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "pads", SourceInstanceID: "t", OriginalID: "x", Domain: "space",
		ObservedUTC: time.Now().UTC(), Payload: payload,
	})
	if err != nil || len(messages) != 1 || messages[0].SemanticType != "space.pad" {
		t.Fatalf("%v %#v", err, messages)
	}
	if err := messages[0].Validate(); err != nil {
		t.Fatal(err)
	}
}
