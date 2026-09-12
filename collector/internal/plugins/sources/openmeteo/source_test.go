package openmeteo

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestRoundCell(t *testing.T) {
	if got := RoundCell(47.52); got != 47.5 {
		t.Fatalf("got %v", got)
	}
}

func TestSampleAndCache(t *testing.T) {
	var calls atomic.Int32
	source, err := New(config.Source{ID: "wx", Type: "weather.openmeteo", Latitude: 47.52, Longitude: 9.21, PollSeconds: 300}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context, float64, float64) ([]byte, error) {
		calls.Add(1)
		return []byte(`{"current":{"time":"2026-09-12T05:00","temperature_2m":18.4,"weather_code":2,"wind_speed_10m":12.3,"wind_direction_10m":240,"relative_humidity_2m":65}}`), nil
	}
	first, err := source.sample(context.Background())
	if err != nil || len(first) != 1 {
		t.Fatalf("first: %v %d", err, len(first))
	}
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one fetch, got %d", calls.Load())
	}
	var payload map[string]any
	_ = json.Unmarshal(first[0].Payload, &payload)
	if payload["attribution"] != "Weather data by Open-Meteo.com" {
		t.Fatalf("attribution %v", payload["attribution"])
	}
}
