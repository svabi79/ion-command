package hapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestRequiresIdentifier(t *testing.T) {
	if _, err := New(config.Source{ID: "h", Type: "humanitarian.hapi"}, slog.Default()); err == nil {
		t.Fatal("expected fail-closed without apiKey")
	}
}

func TestSampleAggregatesHostsAndOrigins(t *testing.T) {
	body, err := os.ReadFile("testdata/refugees.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "h", Type: "humanitarian.hapi", ApiKey: "test-id", PollSeconds: 21600}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.now = func() time.Time { return time.Date(2025, 9, 12, 0, 0, 0, 0, time.UTC) }
	source.fetch = func(context.Context, int) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]int{}
	byID := map[string]map[string]any{}
	for _, record := range records {
		var payload map[string]any
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		roles[payload["role"].(string)]++
		byID[record.OriginalID] = payload
	}
	if roles["host"] < 2 || roles["origin"] < 1 {
		t.Fatalf("roles %v payloads %v", roles, byID)
	}
	if byID["hapi-host-TUR"]["population"].(float64) != 2600000 {
		t.Fatalf("turkey %v", byID["hapi-host-TUR"])
	}
	if _, ok := byID["hapi-host-ALB"]; ok {
		t.Fatal("tiny Albania host should be dropped")
	}
	if byID["hapi-origin-UKR"]["population"].(float64) != 1700000 {
		t.Fatalf("ukraine origin %v", byID["hapi-origin-UKR"])
	}
}

func TestPollFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "humanitarian.hapi", ApiKey: "x", PollSeconds: 60}, slog.Default()); err == nil {
		t.Fatal("expected floor rejection")
	}
}
