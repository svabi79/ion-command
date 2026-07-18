package events

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestEnvelopeValidation(t *testing.T) {
	event := NewEnvelope("test-1", "weather", "weather.lightning", MessageObservation, SourceRef{PluginID: "mock", InstanceID: "test"}, time.Now())
	event.Geometry = Point(8.3, 47.2, 0)
	if err := event.Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	event.Geometry = Point(181, 47.2, 0)
	if err := event.Validate(); err == nil {
		t.Fatal("invalid longitude accepted")
	}
}

func TestRepositorySampleDataMatchesCanonicalContract(t *testing.T) {
	file, err := os.Open("../../../sample-data/demo-spots.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		var event Envelope
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("decode sample line %d: %v", count+1, err)
		}
		if err := event.Validate(); err != nil {
			t.Fatalf("validate sample line %d: %v", count+1, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected three sample messages, got %d", count)
	}
}

func TestUnknownGeometryIsForwardCompatible(t *testing.T) {
	event := NewEnvelope("test-2", "future", "future.thing", MessageField, SourceRef{PluginID: "mock", InstanceID: "test"}, time.Now())
	event.Geometry.Type = "FutureGeometry"
	if err := event.Validate(); err != nil {
		t.Fatalf("unknown geometry should remain forwardable: %v", err)
	}
}

func TestMovingSatelliteTrackUsesGenericEnvelope(t *testing.T) {
	event := NewEnvelope("sat-1", "satellite", "satellite.position", MessageTrack, SourceRef{PluginID: "mock.satellite", InstanceID: "test"}, time.Now())
	event.EntityID = "satellite:25544"
	event.Geometry.Type = "Track"
	event.Geometry.Coordinates = json.RawMessage(`[[8.1,47.0,408000],[8.4,47.2,408100]]`)
	if err := event.Validate(); err != nil {
		t.Fatalf("generic satellite track rejected: %v", err)
	}
}
