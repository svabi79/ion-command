// Package usgs polls the USGS earthquake GeoJSON summary feed
// (https://earthquake.usgs.gov, public domain) and emits one geophysics
// record per new or updated event.
package usgs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	feedURL     = "https://earthquake.usgs.gov/earthquakes/feed/v1.0/summary/all_hour.geojson"
	pollDefault = 2 * time.Minute
	pollFloor   = time.Minute
)

type featureCollection struct {
	Features []struct {
		ID         string `json:"id"`
		Properties struct {
			Mag     *float64 `json:"mag"`
			Place   string   `json:"place"`
			TimeMs  int64    `json:"time"`
			Updated int64    `json:"updated"`
		} `json:"properties"`
		Geometry struct {
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

type Source struct {
	id       string
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger
	// seen dedupes events across the sliding one-hour window; the update
	// timestamp lets edited events through once per revision.
	seen map[string]int64
	// fetch is swappable for tests.
	fetch func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "earthquake.usgs" {
		return nil, fmt.Errorf("unsupported USGS source type %q", sourceConfig.Type)
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("USGS poll interval below one minute (got %s)", interval)
	}
	if logger == nil {
		logger = slog.Default()
	}
	source := &Source{id: sourceConfig.ID, interval: interval, client: &http.Client{Timeout: 20 * time.Second}, logger: logger, seen: make(map[string]int64)}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "earthquake.usgs" }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		records, err := s.sample(ctx)
		if err != nil {
			s.logger.Warn("USGS sample failed", "error", err)
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
		case <-ticker.C:
		}
	}
}

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", feedURL, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 8<<20))
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var collection featureCollection
	if err := json.Unmarshal(body, &collection); err != nil {
		return nil, fmt.Errorf("decode earthquake feed: %w", err)
	}
	// The window slides, so the map stays naturally bounded; prune anyway.
	if len(s.seen) > 4096 {
		s.seen = make(map[string]int64)
	}
	var records []plugins.RawRecord
	for _, feature := range collection.Features {
		if feature.ID == "" || feature.Properties.Mag == nil || len(feature.Geometry.Coordinates) < 2 {
			continue
		}
		if previous, ok := s.seen[feature.ID]; ok && previous >= feature.Properties.Updated {
			continue
		}
		s.seen[feature.ID] = feature.Properties.Updated
		depthKm := 0.0
		if len(feature.Geometry.Coordinates) > 2 {
			depthKm = feature.Geometry.Coordinates[2]
		}
		payload, err := json.Marshal(map[string]any{
			"quakeId":   feature.ID,
			"longitude": feature.Geometry.Coordinates[0],
			"latitude":  feature.Geometry.Coordinates[1],
			"magnitude": *feature.Properties.Mag,
			"depthKm":   depthKm,
			"place":     feature.Properties.Place,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID:   "usgs",
			SourceInstanceID: s.id,
			OriginalID:       fmt.Sprintf("usgs-%s-%s-%d", s.id, feature.ID, feature.Properties.Updated),
			Domain:           "geophysics",
			ObservedUTC:      time.UnixMilli(feature.Properties.TimeMs).UTC(),
			Payload:          payload,
		})
	}
	return records, nil
}
