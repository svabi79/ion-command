// Package coops polls NOAA CO-OPS tide/water-level station metadata as
// Points, distinct from NDBC buoys. Public domain.
package coops

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://api.tidesandcurrents.noaa.gov/mdapi/prod/webapi/stations.json?type=waterlevels"
	pollDefault = 24 * time.Hour
	pollFloor   = 6 * time.Hour
	maxStations = 320
)

type Source struct {
	id       string
	url      string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.coops" {
		return nil, fmt.Errorf("unsupported coops source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("coops poll interval below six hours (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "coops"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "stations.json"), TTL: interval},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.coops" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("coops using disk cache", "error", err)
			return cached, nil
		}
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

type stationDoc struct {
	Stations []struct {
		ID    string  `json:"id"`
		Name  string  `json:"name"`
		State string  `json:"state"`
		Type  string  `json:"type"`
		Lat   float64 `json:"lat"`
		Lng   float64 `json:"lng"`
	} `json:"stations"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	var doc stationDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode coops stations: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxStations)
	for _, station := range doc.Stations {
		if station.ID == "" || (station.Lat == 0 && station.Lng == 0) {
			continue
		}
		kind := strings.TrimSpace(station.Type)
		if kind == "" {
			kind = "waterlevels"
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "tide", "stationId": station.ID, "name": station.Name, "state": station.State,
			"stationKind": kind, "latitude": station.Lat, "longitude": station.Lng,
			"attribution": "NOAA CO-OPS (public domain)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "coops", SourceInstanceID: s.id, OriginalID: "coops-" + station.ID,
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxStations {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("coops sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("coops snapshot", "source", s.id, "stations", len(records))
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		if err := pollutil.Sleep(ctx, s.interval); err != nil {
			return nil
		}
	}
}
