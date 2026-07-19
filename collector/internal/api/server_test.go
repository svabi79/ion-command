package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/pipeline"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/recording"
	"github.com/ion-command/ion-command/collector/internal/stream"
	"github.com/ion-command/ion-command/collector/internal/telemetry"
)

func TestHealthEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.Recording.Directory = t.TempDir()
	stats := telemetry.New()
	hub := stream.NewHub(8, stats)
	registry := plugins.NewRegistry()
	pipe := pipeline.New(registry, 8, 1, hub, recording.New(cfg.Recording.Directory, false, time.Second), stats, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	server := httptest.NewServer(New(cfg, pipe, hub, stats, slog.Default()).Handler())
	defer server.Close()
	response, err := http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", response.StatusCode)
	}
}

func TestLiveWebSocketReceivesCanonicalMessage(t *testing.T) {
	cfg := config.Default()
	cfg.Recording.Directory = t.TempDir()
	stats := telemetry.New()
	hub := stream.NewHub(8, stats)
	registry := plugins.NewRegistry()
	pipe := pipeline.New(registry, 8, 1, hub, recording.New(cfg.Recording.Directory, false, time.Second), stats, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	server := httptest.NewServer(New(cfg, pipe, hub, stats, slog.Default()).Handler())
	defer server.Close()
	url := "ws" + server.URL[len("http"):] + "/ws/live"
	connection, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	deadline := time.Now().Add(time.Second)
	for stats.Snapshot().ConnectedClients == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	hub.Publish([]byte(`{"messageId":"one"}`), "")
	_ = connection.SetReadDeadline(time.Now().Add(time.Second))
	_, payload, err := connection.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != `{"messageId":"one"}` {
		t.Fatalf("unexpected payload: %s", payload)
	}
}
