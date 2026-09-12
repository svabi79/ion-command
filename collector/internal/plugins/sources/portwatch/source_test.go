package portwatch

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleJoinsDailyCountsAndRecentDisruptions(t *testing.T) {
	disruptionBody, err := os.ReadFile("testdata/disruptions.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "p", Type: "maritime.portwatch"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context, rawURL string) ([]byte, error) {
		if strings.Contains(rawURL, "Daily") {
			return []byte(`{"features":[{"attributes":{"portid":"chokepoint1","n_total":42}}]}`), nil
		}
		if strings.Contains(rawURL, "disruptions") {
			return disruptionBody, nil
		}
		return []byte(`{"features":[{"attributes":{"portid":"chokepoint1","portname":"Suez Canal"},"geometry":{"x":32.3,"y":30.5}}]}`), nil
	}
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
	var payload map[string]any
	if err := json.Unmarshal(records[1].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "disruption" || payload["eventId"] != "1000999" {
		t.Fatalf("%v", payload)
	}
}

func TestRecentDisruptionKeepsOpenAndDropsStale(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	recent := float64(now.Add(-2 * 24 * time.Hour).UnixMilli())
	stale := float64(now.Add(-400 * 24 * time.Hour).UnixMilli())
	if !recentDisruption(map[string]any{"fromdate": recent}, now) {
		t.Fatal("recent open-ended event should stay")
	}
	if recentDisruption(map[string]any{"fromdate": stale, "todate": stale}, now) {
		t.Fatal("2019-style event should drop")
	}
}
