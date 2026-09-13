package grayline

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestNewRejectsWrongType(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "solar.other"}, slog.Default()); err == nil {
		t.Fatal("expected type rejection")
	}
}

func TestRecordAtSolstice(t *testing.T) {
	at := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	record, ok := RecordAt(at, "test")
	if !ok {
		t.Fatal("expected a grayline record")
	}
	if record.Domain != "solar" {
		t.Fatalf("unexpected domain %q", record.Domain)
	}
	var payload struct {
		Kind  string        `json:"kind"`
		Rings [][][]float64 `json:"rings"`
	}
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Kind != "grayline" || len(payload.Rings) != 1 || len(payload.Rings[0]) < 16 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestPollFloor(t *testing.T) {
	source, err := New(config.Source{ID: "g", Type: "solar.grayline", PollSeconds: 1}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if source.interval < pollFloor {
		t.Fatalf("poll floor not applied: %s", source.interval)
	}
}
