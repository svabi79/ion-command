package replay

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/recording"
)

func TestReplayPreservesOrder(t *testing.T) {
	directory := t.TempDir()
	writer := recording.New(directory, true, time.Millisecond)
	base := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		e := events.NewEnvelope(string(rune('a'+i)), "weather", "weather.lightning", events.MessageObservation, events.SourceRef{PluginID: "test", InstanceID: "test"}, base.Add(time.Duration(i)*time.Millisecond))
		data, _ := json.Marshal(e)
		if err := writer.Write(e.Time.ObservedUTC, data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0)
	err := Stream(context.Background(), directory, time.Time{}, time.Time{}, 1000, func(line []byte) error {
		var e events.Envelope
		_ = json.Unmarshal(line, &e)
		ids = append(ids, e.MessageID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != "a" || ids[2] != "c" {
		t.Fatalf("unexpected order: %v", ids)
	}
}

func TestReplayPreservesUnknownSemanticType(t *testing.T) {
	directory := t.TempDir()
	writer := recording.New(directory, true, time.Millisecond)
	observed := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	event := events.NewEnvelope("future-1", "future", "future.uninstalled.sensor", events.MessageObservation, events.SourceRef{PluginID: "external.future", InstanceID: "test"}, observed)
	event.Geometry = events.Point(8.3, 47.2, 0)
	event.Properties["opaqueValue"] = map[string]any{"nested": true}
	written, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(observed, written); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var replayed []byte
	if err := Stream(context.Background(), directory, time.Time{}, time.Time{}, 1000, func(line []byte) error {
		replayed = append([]byte(nil), line...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if string(replayed) != string(written) {
		t.Fatalf("unknown semantic message changed during replay\nwant: %s\n got: %s", written, replayed)
	}
}
