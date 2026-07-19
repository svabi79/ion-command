package stream

import (
	"testing"

	"github.com/ion-command/ion-command/collector/internal/telemetry"
)

func TestRetainedStateReplaysToNewClients(t *testing.T) {
	hub := NewHub(16, telemetry.New())
	hub.Publish([]byte(`{"v":"old"}`), "spaceweather.state|")
	hub.Publish([]byte(`{"v":"new"}`), "spaceweather.state|")
	hub.Publish([]byte(`{"v":"ea036"}`), "ionosphere.sounding|ionosphere:station:EA036")
	hub.Publish([]byte(`{"v":"transient"}`), "")

	client := hub.Register()
	defer hub.Unregister(client)
	received := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case message := <-client.Messages:
			received[string(message)] = true
		default:
			t.Fatalf("expected two retained snapshots, got %d", len(received))
		}
	}
	if !received[`{"v":"new"}`] || !received[`{"v":"ea036"}`] {
		t.Fatalf("unexpected snapshot set: %v", received)
	}
	select {
	case extra := <-client.Messages:
		t.Fatalf("transient message must not be retained: %s", extra)
	default:
	}
}

func TestRetainedStateIsBounded(t *testing.T) {
	hub := NewHub(1, telemetry.New())
	for i := 0; i < maxRetainedMessages+50; i++ {
		hub.Publish([]byte(`{}`), string(rune('a'+i%26))+string(rune('0'+i%10))+string(rune(i)))
	}
	if len(hub.retained) > maxRetainedMessages {
		t.Fatalf("retained map exceeded cap: %d", len(hub.retained))
	}
}
