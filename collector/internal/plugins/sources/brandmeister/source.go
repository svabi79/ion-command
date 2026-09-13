// Package brandmeister streams DMR last-heard activity from the public
// Brandmeister Socket.IO feed (https://api.brandmeister.network, path
// /lh/socket.io). No credentials. One connection, exponential reconnect
// backoff, and a local emit throttle so a busy talkgroup cannot flood the
// pipeline. Positions come from the AD1C country file (entity centroids).
package brandmeister

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/cty"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	defaultURL     = "wss://api.brandmeister.network/lh/socket.io/?EIO=4&transport=websocket"
	reconnectFloor = 10 * time.Second
	reconnectMax   = 5 * time.Minute
	healthyReset   = 2 * time.Minute
	readIdle       = 90 * time.Second
	// Global last-heard is chatty; keep hobby-scale.
	emitMinInterval = 150 * time.Millisecond
	minDurationS    = 1
	maxTalkgroup    = 999999
)

type Source struct {
	id       string
	endpoint string
	logger   *slog.Logger
	now      func() time.Time
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "hamradio.brandmeister" {
		return nil, fmt.Errorf("unsupported brandmeister source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	endpoint := defaultURL
	if sourceConfig.Broker != "" {
		endpoint = sourceConfig.Broker
		if !strings.Contains(endpoint, "EIO=") {
			u, err := url.Parse(endpoint)
			if err == nil {
				q := u.Query()
				if q.Get("EIO") == "" {
					q.Set("EIO", "4")
				}
				if q.Get("transport") == "" {
					q.Set("transport", "websocket")
				}
				u.RawQuery = q.Encode()
				endpoint = u.String()
			}
		}
	}
	return &Source{id: sourceConfig.ID, endpoint: endpoint, logger: logger, now: time.Now}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "hamradio.brandmeister" }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	backoff := reconnectFloor
	for ctx.Err() == nil {
		connectedAt := time.Now()
		err := s.stream(ctx, output)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			s.logger.Warn("brandmeister last-heard ended", "source", s.id, "error", err)
		}
		if time.Since(connectedAt) >= healthyReset {
			backoff = reconnectFloor
		} else {
			backoff = min(backoff*2, reconnectMax)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
	}
	return nil
}

func (s *Source) stream(ctx context.Context, output chan<- plugins.RawRecord) error {
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, s.endpoint, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	opened := false
	joined := false
	lastEmit := time.Time{}
	for {
		conn.SetReadDeadline(time.Now().Add(readIdle))
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		kind, body := splitEngineIO(message)
		switch kind {
		case '2':
			_ = conn.WriteMessage(websocket.TextMessage, []byte("3"))
		case '0':
			if !opened {
				if err := conn.WriteMessage(websocket.TextMessage, []byte("40")); err != nil {
					return fmt.Errorf("socket.io connect: %w", err)
				}
				opened = true
			}
		case '4':
			if !joined && isNamespaceOpen(body) {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(`42["join","everything"]`)); err != nil {
					return fmt.Errorf("join: %w", err)
				}
				joined = true
				s.logger.Info("brandmeister last-heard connected", "source", s.id)
			}
			payload, ok := extractMQTT(body)
			if !ok {
				continue
			}
			record, ok := RecordFromLastHeard(payload, s.id, s.now().UTC())
			if !ok {
				continue
			}
			if !lastEmit.IsZero() && s.now().Sub(lastEmit) < emitMinInterval {
				continue
			}
			lastEmit = s.now()
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func splitEngineIO(message []byte) (byte, []byte) {
	if len(message) == 0 {
		return 0, nil
	}
	return message[0], message[1:]
}

func isNamespaceOpen(body []byte) bool {
	return len(body) > 0 && body[0] == '0'
}

func extractMQTT(body []byte) (json.RawMessage, bool) {
	// Socket.IO EVENT is "2[...]".
	if len(body) < 3 || body[0] != '2' {
		return nil, false
	}
	var packet []json.RawMessage
	if err := json.Unmarshal(body[1:], &packet); err != nil || len(packet) < 2 {
		return nil, false
	}
	var name string
	if err := json.Unmarshal(packet[0], &name); err != nil || name != "mqtt" {
		return nil, false
	}
	return packet[1], true
}

type lastHeard struct {
	Event           string `json:"Event"`
	SourceCall      string `json:"SourceCall"`
	SourceName      string `json:"SourceName"`
	DestinationID   int64  `json:"DestinationID"`
	DestinationName string `json:"DestinationName"`
	Start           int64  `json:"Start"`
	Stop            int64  `json:"Stop"`
	TalkerAlias     string `json:"TalkerAlias"`
	SessionType     int64  `json:"SessionType"`
}

// RecordFromLastHeard turns one Brandmeister last-heard object (or a
// wrapper with a JSON/string "payload" field) into a hamradio activity
// record. Group voice completions only; unplaceable callsigns are dropped.
func RecordFromLastHeard(raw json.RawMessage, sourceInstanceID string, observed time.Time) (plugins.RawRecord, bool) {
	payload, ok := unwrapPayload(raw)
	if !ok {
		return plugins.RawRecord{}, false
	}
	var heard lastHeard
	if err := json.Unmarshal(payload, &heard); err != nil {
		return plugins.RawRecord{}, false
	}
	if !isCompletedGroup(heard) {
		return plugins.RawRecord{}, false
	}
	callsign := strings.ToUpper(strings.TrimSpace(heard.SourceCall))
	if callsign == "" {
		return plugins.RawRecord{}, false
	}
	duration := heard.Stop - heard.Start
	if heard.Stop > 0 && duration < minDurationS {
		return plugins.RawRecord{}, false
	}
	entity, ok := cty.Lookup(callsign)
	if !ok {
		return plugins.RawRecord{}, false
	}
	spotID := fmt.Sprintf("bm-%s-%d-%d", callsign, heard.DestinationID, heard.Start)
	if heard.Start == 0 {
		spotID = fmt.Sprintf("bm-%s-%d-%d", callsign, heard.DestinationID, observed.Unix())
	}
	body, err := json.Marshal(map[string]any{
		"kind":          "last-heard",
		"spotId":        spotID,
		"txCallsign":    callsign,
		"txLongitude":   entity.Longitude,
		"txLatitude":    entity.Latitude,
		"txRegion":      entity.Name,
		"talkgroup":     heard.DestinationID,
		"talkgroupName": heard.DestinationName,
		"durationS":     duration,
		"sourceName":    heard.SourceName,
		"talkerAlias":   heard.TalkerAlias,
		"mode":          "DMR",
	})
	if err != nil {
		return plugins.RawRecord{}, false
	}
	return plugins.RawRecord{
		SourcePluginID:   "brandmeister",
		SourceInstanceID: sourceInstanceID,
		OriginalID:       spotID,
		Domain:           "hamradio",
		ObservedUTC:      observed,
		Payload:          body,
	}, true
}

func unwrapPayload(raw json.RawMessage) (json.RawMessage, bool) {
	var wrapper struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Payload) > 0 {
		if wrapper.Payload[0] == '"' {
			var inner string
			if err := json.Unmarshal(wrapper.Payload, &inner); err == nil {
				return json.RawMessage(inner), true
			}
		}
		return wrapper.Payload, true
	}
	return raw, len(raw) > 0
}

func isCompletedGroup(heard lastHeard) bool {
	event := strings.ToLower(heard.Event)
	if !strings.Contains(event, "session-stop") && !strings.Contains(event, "session-end") {
		return false
	}
	if heard.DestinationID <= 0 || heard.DestinationID > maxTalkgroup {
		return false
	}
	return true
}
