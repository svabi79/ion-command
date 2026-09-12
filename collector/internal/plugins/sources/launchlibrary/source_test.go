package launchlibrary

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleParsesUpcoming(t *testing.T) {
	body, err := os.ReadFile("testdata/upcoming.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "ll", Type: "space.launchlibrary", PollSeconds: 900}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected the 30-day launch only, got %d", len(records))
	}
	var payload map[string]any
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["padName"] != "SLC-40" {
		t.Fatalf("pad %v", payload["padName"])
	}
}

func TestPollFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "space.launchlibrary", PollSeconds: 60}, slog.Default()); err == nil {
		t.Fatal("expected floor rejection")
	}
}
