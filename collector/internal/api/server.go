package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/pipeline"
	"github.com/ion-command/ion-command/collector/internal/replay"
	"github.com/ion-command/ion-command/collector/internal/stream"
	"github.com/ion-command/ion-command/collector/internal/telemetry"
)

const BuildVersion = "0.9.1"

type Server struct {
	config   config.Config
	pipeline *pipeline.Pipeline
	hub      *stream.Hub
	stats    *telemetry.Stats
	logger   *slog.Logger
	started  time.Time
	upgrader websocket.Upgrader
}

func New(cfg config.Config, pipeline *pipeline.Pipeline, hub *stream.Hub, stats *telemetry.Stats, logger *slog.Logger) *Server {
	return &Server{config: cfg, pipeline: pipeline, hub: hub, stats: stats, logger: logger, started: time.Now(), upgrader: websocket.Upgrader{ReadBufferSize: 4096, WriteBufferSize: 4096, CheckOrigin: func(_ *http.Request) bool { return true }}}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/stats", s.statsHandler)
	mux.HandleFunc("GET /api/sources", s.sources)
	mux.HandleFunc("GET /api/replay/range", s.replayRange)
	mux.HandleFunc("GET /ws/live", s.live)
	mux.HandleFunc("GET /ws/replay", s.replay)
	return withHeaders(mux)
}

func (s *Server) HTTPServer() *http.Server {
	return &http.Server{Addr: s.config.Server.ListenAddress, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: time.Duration(s.config.Server.WriteTimeoutSeconds) * time.Second}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "utc": time.Now().UTC()})
}
func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"productName": s.config.Branding.ProductName, "version": BuildVersion, "uptimeSeconds": int64(time.Since(s.started).Seconds()), "recording": s.config.Recording.Enabled})
}
func (s *Server) statsHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.stats.Snapshot())
}
func (s *Server) sources(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.pipeline.Sources())
}
func (s *Server) replayRange(w http.ResponseWriter, _ *http.Request) {
	available, err := replay.AvailableRange(s.config.Recording.Directory)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, available)
}

func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	connection, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := s.hub.Register()
	defer s.hub.Unregister(client)
	defer connection.Close()
	connection.SetReadLimit(1024)
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := connection.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-closed:
			return
		case message, ok := <-client.Messages:
			if !ok {
				return
			}
			_ = connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := connection.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ping.C:
			_ = connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request) {
	connection, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()
	from, err := parseTime(r.URL.Query().Get("from"))
	if err != nil {
		_ = connection.WriteJSON(map[string]string{"error": err.Error()})
		return
	}
	to, err := parseTime(r.URL.Query().Get("to"))
	if err != nil {
		_ = connection.WriteJSON(map[string]string{"error": err.Error()})
		return
	}
	speed := 1.0
	if value := r.URL.Query().Get("speed"); value != "" {
		speed, err = strconv.ParseFloat(value, 64)
		if err != nil || speed <= 0 {
			_ = connection.WriteJSON(map[string]string{"error": "speed must be positive"})
			return
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	err = replay.Stream(ctx, s.config.Recording.Directory, from, to, speed, func(line []byte) error {
		_ = connection.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return connection.WriteMessage(websocket.TextMessage, line)
	})
	if err != nil {
		s.logger.Warn("replay stream stopped", "error", err)
	}
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func withHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
