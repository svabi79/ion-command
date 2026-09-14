// Package usgsvolcano polls USGS volcano status GeoJSON (US observatories).
// HANS/VONA list endpoint was not a clean public JSON in 2026-09 (404 on
// getRecent); alertLevel/colorCode on this feed already carry the status.
package usgsvolcano

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/geoutil"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL   = "https://volcanoes.usgs.gov/vsc/api/volcanoApi/geojson"
	pollDefault  = 15 * time.Minute
	pollFloor    = 5 * time.Minute
	maxVolcanoes = 200
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
	if sourceConfig.Type != "geophysics.usgsvolcano" {
		return nil, fmt.Errorf("unsupported usgsvolcano source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("usgsvolcano poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "usgsvolcano"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "volcanoes.json"), TTL: interval},
		client: &http.Client{Timeout: 40 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geophysics.usgsvolcano" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, _, ok := s.cache.Load(); ok {
		s.logger.Warn("usgsvolcano using disk cache", "error", err)
		return cached, nil
	}
	return nil, err
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := geoutil.ParseFeatureCollection(body)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxVolcanoes)
	for _, feature := range collection.Features {
		lon, lat, ok := geoutil.Point(feature.Geometry)
		if !ok {
			continue
		}
		id := geoutil.PropertyString(feature.Properties, "vnum", "volcanoCd")
		if id == "" {
			id = geoutil.FeatureID(feature)
		}
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "volcano", "volcanoId": id,
			"name":     geoutil.PropertyString(feature.Properties, "volcanoName"),
			"alert":    geoutil.PropertyString(feature.Properties, "alertLevel"),
			"color":    geoutil.PropertyString(feature.Properties, "colorCode"),
			"region":   geoutil.PropertyString(feature.Properties, "region"),
			"latitude": lat, "longitude": lon, "provider": "usgs",
			"attribution": "U.S. Geological Survey (public domain)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "usgsvolcano", SourceInstanceID: s.id, OriginalID: "usgsvolcano-" + id,
			Domain: "geophysics", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxVolcanoes {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("usgsvolcano sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("usgsvolcano snapshot", "source", s.id, "volcanoes", len(records))
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
