package brandmeister

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestNewRejectsWrongType(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "hamradio.other"}, slog.Default()); err == nil {
		t.Fatal("expected type rejection")
	}
}

func TestFixtureSessionStopBecomesActivity(t *testing.T) {
	body, err := os.ReadFile("testdata/lastheard_mqtt.json")
	if err != nil {
		t.Fatal(err)
	}
	record, ok := RecordFromLastHeard(body, "test", time.Unix(1722470414, 0).UTC())
	if !ok {
		t.Fatal("fixture last-heard must produce a record")
	}
	if record.Domain != "hamradio" {
		t.Fatalf("domain %q", record.Domain)
	}
	var payload map[string]any
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "last-heard" || payload["txCallsign"] != "HB9HSJ" {
		t.Fatalf("unexpected payload: %v", payload)
	}
	if payload["talkgroup"].(float64) != 91 {
		t.Fatalf("talkgroup %v", payload["talkgroup"])
	}
	if payload["mode"] != "DMR" {
		t.Fatalf("mode %v", payload["mode"])
	}
	if payload["txLatitude"].(float64) == 0 && payload["txLongitude"].(float64) == 0 {
		t.Fatal("CTY must place HB9HSJ")
	}
}

func TestSkipsKerchunkAndPrivateCall(t *testing.T) {
	kerchunk, _ := json.Marshal(map[string]any{
		"Event": "Session-Stop", "SourceCall": "HB9HSJ", "DestinationID": 91,
		"Start": 100, "Stop": 100,
	})
	if _, ok := RecordFromLastHeard(kerchunk, "t", time.Unix(100, 0).UTC()); ok {
		t.Fatal("sub-second kerchunk must be dropped")
	}
	private, _ := json.Marshal(map[string]any{
		"Event": "Session-Stop", "SourceCall": "HB9HSJ", "DestinationID": 2283011,
		"Start": 100, "Stop": 120,
	})
	if _, ok := RecordFromLastHeard(private, "t", time.Unix(120, 0).UTC()); ok {
		t.Fatal("7-digit private destination must be dropped")
	}
	start, _ := json.Marshal(map[string]any{
		"Event": "Session-Start", "SourceCall": "HB9HSJ", "DestinationID": 91,
		"Start": 100, "Stop": 0,
	})
	if _, ok := RecordFromLastHeard(start, "t", time.Unix(100, 0).UTC()); ok {
		t.Fatal("Session-Start must wait for completion")
	}
}

func TestExtractMQTTPacket(t *testing.T) {
	packet := []byte(`42["mqtt",{"payload":"{\"Event\":\"Session-Stop\",\"SourceCall\":\"K1ABC\",\"DestinationID\":91,\"Start\":1,\"Stop\":10}"}]`)
	kind, body := splitEngineIO(packet)
	if kind != '4' {
		t.Fatalf("engine.io kind %c", kind)
	}
	raw, ok := extractMQTT(body)
	if !ok {
		t.Fatal("expected mqtt event")
	}
	record, ok := RecordFromLastHeard(raw, "t", time.Unix(10, 0).UTC())
	if !ok {
		t.Fatal("mqtt payload should decode")
	}
	var payload map[string]any
	_ = json.Unmarshal(record.Payload, &payload)
	if payload["txCallsign"] != "K1ABC" {
		t.Fatalf("callsign %v", payload["txCallsign"])
	}
}

func TestExtendedEventNameAccepted(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"Event": "Session-Stop-Extended", "SourceCall": "W1AW", "DestinationID": 3100,
		"DestinationName": "USA", "Start": 50, "Stop": 65,
	})
	if _, ok := RecordFromLastHeard(body, "t", time.Unix(65, 0).UTC()); !ok {
		t.Fatal("Session-Stop-Extended is the current Brandmeister event name")
	}
}
