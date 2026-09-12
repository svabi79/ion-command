package openaq

import (
	"context"
	"log/slog"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestRequiresKey(t *testing.T) {
	if _, err := New(config.Source{ID: "a", Type: "weather.openaq", Latitude: 1, Longitude: 2}, slog.Default()); err == nil {
		t.Fatal("expected apiKey rejection")
	}
}

func TestSample(t *testing.T) {
	source, err := New(config.Source{ID: "a", Type: "weather.openaq", ApiKey: "k", Latitude: 47.5, Longitude: 8.5}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context) ([]byte, error) {
		return []byte(`{"results":[{"id":11,"name":"Zurich","coordinates":{"latitude":47.4,"longitude":8.5},"country":{"name":"Switzerland"},"sensors":[{"parameter":{"name":"pm25","displayName":"PM2.5"}}]}]}`), nil
	}
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
}
