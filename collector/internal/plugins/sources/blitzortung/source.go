package blitzortung

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

// The community websocket fan-out hosts; rotated on failure.
var defaultHosts = []string{"wss://ws1.blitzortung.org/", "wss://ws7.blitzortung.org/", "wss://ws8.blitzortung.org/"}

const subscribeMessage = `{"a": 111}`

type Source struct {
	id     string
	hosts  []string
	logger *slog.Logger
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "lightning.blitzortung" {
		return nil, fmt.Errorf("unsupported blitzortung source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	hosts := defaultHosts
	if sourceConfig.Broker != "" {
		hosts = []string{sourceConfig.Broker}
	}
	return &Source{id: sourceConfig.ID, hosts: hosts, logger: logger}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "lightning.blitzortung" }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	hostIndex := 0
	for ctx.Err() == nil {
		host := s.hosts[hostIndex%len(s.hosts)]
		hostIndex++
		if err := s.stream(ctx, host, output); err != nil && ctx.Err() == nil {
			s.logger.Warn("blitzortung stream ended", "host", host, "error", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
	return nil
}

func (s *Source) stream(ctx context.Context, host string, output chan<- plugins.RawRecord) error {
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, host, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", host, err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(subscribeMessage)); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	s.logger.Info("blitzortung connected", "host", host)
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	invalidFrames := 0
	for {
		// Strikes pause during global lulls; a generous deadline detects a
		// dead connection without churning through healthy quiet spells.
		conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		record, ok, err := ParseStrike(message, s.id)
		if err != nil {
			invalidFrames++
			if invalidFrames <= 3 || invalidFrames%1000 == 0 {
				s.logger.Warn("blitzortung frame rejected", "error", err, "count", invalidFrames)
			}
			continue
		}
		if !ok {
			continue
		}
		select {
		case output <- record:
		case <-ctx.Done():
			return nil
		}
	}
}
