// Package gdacs polls the Global Disaster Alert and Coordination System
// event list (https://www.gdacs.org) and emits one record per alert.
package gdacs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://www.gdacs.org/gdacsapi/api/events/geteventlist/SEARCH?eventlist=EQ;TC;FL;VO;DR;WF;TS&alertlevel=Green;Orange;Red"
	pollDefault = 10 * time.Minute
	pollFloor   = 5 * time.Minute
	maxEvents   = 100
)

type Source struct {
	id       string
	url      string
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "geophysics.gdacs" {
		return nil, fmt.Errorf("unsupported gdacs source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("gdacs poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	source := &Source{id: sourceConfig.ID, url: url, interval: interval, client: &http.Client{Timeout: 40 * time.Second}, logger: logger}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geophysics.gdacs" }

type gdacsResponse struct {
	Features []gdacsFeature `json:"features"`
}

type gdacsFeature struct {
	Properties map[string]any `json:"properties"`
	Geometry   struct {
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func firstPoint(raw json.RawMessage) (lon, lat float64, ok bool) {
	var point []float64
	if json.Unmarshal(raw, &point) == nil && len(point) >= 2 {
		return point[0], point[1], true
	}
	var nested [][]float64
	if json.Unmarshal(raw, &nested) == nil && len(nested) > 0 && len(nested[0]) >= 2 {
		return nested[0][0], nested[0][1], true
	}
	return 0, 0, false
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var response gdacsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode gdacs response: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.Features))
	for _, feature := range response.Features {
		lon, lat, ok := firstPoint(feature.Geometry.Coordinates)
		if !ok {
			continue
		}
		eventID := asString(feature.Properties["eventid"])
		if eventID == "" || eventID == "<nil>" {
			eventID = asString(feature.Properties["eventId"])
		}
		if eventID == "" || eventID == "<nil>" {
			continue
		}
		eventType := asString(feature.Properties["eventtype"])
		if eventType == "<nil>" {
			eventType = asString(feature.Properties["eventType"])
		}
		name := asString(feature.Properties["name"])
		if name == "" || name == "<nil>" {
			name = asString(feature.Properties["eventname"])
		}
		alert := asString(feature.Properties["alertlevel"])
		if alert == "<nil>" {
			alert = asString(feature.Properties["alertLevel"])
		}
		payload, err := json.Marshal(map[string]any{
			"kind":       "event",
			"eventId":    "gdacs-" + eventType + "-" + eventID,
			"title":      name,
			"category":   eventType,
			"alertLevel": alert,
			"longitude":  lon,
			"latitude":   lat,
			"provider":   "gdacs",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "gdacs", SourceInstanceID: s.id,
			OriginalID: "gdacs-" + eventType + "-" + eventID,
			Domain: "geophysics", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxEvents {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("gdacs sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("gdacs snapshot", "source", s.id, "events", len(records))
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.interval):
		}
	}
}
