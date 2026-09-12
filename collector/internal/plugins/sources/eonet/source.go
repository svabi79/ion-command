// Package eonet polls NASA's Earth Observatory Natural Event Tracker
// (https://eonet.gsfc.nasa.gov) for currently open natural events.
package eonet

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
	defaultURL  = "https://eonet.gsfc.nasa.gov/api/v3/events?status=open&limit=80"
	pollDefault = 15 * time.Minute
	pollFloor   = 5 * time.Minute
	maxEvents   = 80
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
	if sourceConfig.Type != "geophysics.eonet" {
		return nil, fmt.Errorf("unsupported eonet source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("eonet poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	source := &Source{id: sourceConfig.ID, url: url, interval: interval, client: &http.Client{Timeout: 30 * time.Second}, logger: logger}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geophysics.eonet" }

type eonetResponse struct {
	Events []struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Categories  []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"categories"`
		Geometry []struct {
			Date        string    `json:"date"`
			Type        string    `json:"type"`
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"events"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var response eonetResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode eonet response: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.Events))
	for _, event := range response.Events {
		if event.ID == "" || len(event.Geometry) == 0 {
			continue
		}
		geom := event.Geometry[len(event.Geometry)-1]
		if len(geom.Coordinates) < 2 {
			continue
		}
		category, categoryID := "", ""
		if len(event.Categories) > 0 {
			category = event.Categories[0].Title
			categoryID = event.Categories[0].ID
		}
		payload, err := json.Marshal(map[string]any{
			"kind":        "event",
			"eventId":     event.ID,
			"title":       event.Title,
			"description": event.Description,
			"category":    category,
			"categoryId":  categoryID,
			"longitude":   geom.Coordinates[0],
			"latitude":    geom.Coordinates[1],
			"eventDate":   geom.Date,
			"provider":    "eonet",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "eonet", SourceInstanceID: s.id, OriginalID: event.ID,
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
			s.logger.Warn("eonet sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("eonet snapshot", "source", s.id, "events", len(records))
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
