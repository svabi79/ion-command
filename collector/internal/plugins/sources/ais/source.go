package ais

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	defaultEndpoint = "wss://stream.aisstream.io/v0/stream"
	// reconnectFloor/reconnectMax bound the backoff between reconnect
	// attempts, doubling on each consecutive failure. aisstream.io caps a
	// connecting client at three open sockets per IP and three subscribed
	// connections per account (see docs/DATA-SOURCES.md); a flat short retry
	// on a persistent failure (bad key, network outage) would burn through
	// that budget and could look like abuse.
	reconnectFloor = 10 * time.Second
	reconnectMax   = 5 * time.Minute
	// A connection that stayed up this long is treated as healthy and resets
	// the backoff, so a single brief blip does not leave the source crawling
	// back up from the floor for the rest of the session.
	healthyConnectionResetAfter = 2 * time.Minute
	// readIdleTimeout is generous relative to the AIS reporting-interval
	// table (Class A/B vessels report at least every 3 minutes even at
	// anchor); a socket silent for longer than this is presumed dead rather
	// than a genuine lull, which is not true of AIS traffic the way it can
	// be of, say, global lightning strikes.
	readIdleTimeout = 3 * time.Minute
)

// subscribedMessageTypes are the only AIS message types this source ever
// asks aisstream.io for. Position reports place a vessel on the globe; the
// two static/voyage types feed the name/type/destination the domain joins
// onto those positions. Base stations, aids to navigation, SAR aircraft and
// every other AIS message type are intentionally never subscribed to.
var subscribedMessageTypes = []string{"PositionReport", "StandardClassBPositionReport", "ShipStaticData", "StaticDataReport"}

type Source struct {
	id            string
	endpoint      string
	apiKey        string
	boundingBoxes []config.BoundingBox
	logger        *slog.Logger
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "ais.aisstream" {
		return nil, fmt.Errorf("unsupported ais source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(sourceConfig.ApiKey) == "" {
		return nil, fmt.Errorf("ais.aisstream source requires apiKey (see docs/DATA-SOURCES.md for how to obtain a free key)")
	}
	if len(sourceConfig.BoundingBoxes) == 0 {
		return nil, fmt.Errorf("ais.aisstream source requires at least one entry in boundingBoxes")
	}
	for i, box := range sourceConfig.BoundingBoxes {
		if box.MinLatitude < -90 || box.MaxLatitude > 90 || box.MinLongitude < -180 || box.MaxLongitude > 180 {
			return nil, fmt.Errorf("boundingBoxes[%d] outside WGS84 bounds", i)
		}
		if box.MinLatitude >= box.MaxLatitude || box.MinLongitude >= box.MaxLongitude {
			return nil, fmt.Errorf("boundingBoxes[%d] requires min < max on both axes", i)
		}
	}
	endpoint := defaultEndpoint
	if sourceConfig.Broker != "" {
		endpoint = sourceConfig.Broker
	}
	return &Source{
		id:            sourceConfig.ID,
		endpoint:      endpoint,
		apiKey:        sourceConfig.ApiKey,
		boundingBoxes: sourceConfig.BoundingBoxes,
		logger:        logger,
	}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "ais.aisstream" }

// subscribeMessage builds the JSON aisstream.io expects on connect: an API
// key, one or more [[minLat,minLon],[maxLat,maxLon]] bounding boxes, and the
// message-type filter. The whole thing must reach the server within three
// seconds of the WebSocket handshake completing, so the caller sends it
// before doing anything else.
func (s *Source) subscribeMessage() ([]byte, error) {
	boxes := make([][][2]float64, 0, len(s.boundingBoxes))
	for _, box := range s.boundingBoxes {
		boxes = append(boxes, [][2]float64{
			{box.MinLatitude, box.MinLongitude},
			{box.MaxLatitude, box.MaxLongitude},
		})
	}
	return json.Marshal(map[string]any{
		"APIKey":             s.apiKey,
		"BoundingBoxes":      boxes,
		"FilterMessageTypes": subscribedMessageTypes,
	})
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	backoff := reconnectFloor
	for ctx.Err() == nil {
		connectedAt := time.Now()
		err := s.stream(ctx, output)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			s.logger.Warn("ais stream ended", "source", s.id, "error", err)
		}
		if time.Since(connectedAt) >= healthyConnectionResetAfter {
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
	// permessage-deflate is explicitly recommended by the provider to keep
	// bandwidth down; gorilla negotiates it transparently when the server
	// supports it and falls back cleanly when it does not.
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second, EnableCompression: true}
	conn, _, err := dialer.DialContext(ctx, s.endpoint, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", s.endpoint, err)
	}
	defer conn.Close()
	subscribe, err := s.subscribeMessage()
	if err != nil {
		return fmt.Errorf("build subscription: %w", err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, subscribe); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	s.logger.Info("ais connected", "source", s.id, "endpoint", s.endpoint, "boundingBoxes", len(s.boundingBoxes))
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	invalidFrames := 0
	for {
		conn.SetReadDeadline(time.Now().Add(readIdleTimeout))
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		records, err := ParseFrame(message, s.id)
		if err != nil {
			invalidFrames++
			if invalidFrames <= 3 || invalidFrames%1000 == 0 {
				s.logger.Warn("ais frame rejected", "source", s.id, "error", err, "count", invalidFrames)
			}
			continue
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
	}
}
