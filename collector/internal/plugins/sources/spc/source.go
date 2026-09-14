// Package spc polls NOAA Storm Prediction Center categorical convective
// outlook GeoJSON (day 1/2/3). Public domain.
package spc

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
	pollDefault = 15 * time.Minute
	pollFloor   = 5 * time.Minute
	maxAreas    = 12
	minVertexKm = 25.0
	maxVertices = 40
)

var defaultURLs = []string{
	"https://www.spc.noaa.gov/products/outlook/day1otlk_cat.lyr.geojson",
	"https://www.spc.noaa.gov/products/outlook/day2otlk_cat.lyr.geojson",
	"https://www.spc.noaa.gov/products/outlook/day3otlk_cat.lyr.geojson",
}

type Source struct {
	id       string
	urls     []string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context, rawURL string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.spc" {
		return nil, fmt.Errorf("unsupported spc source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("spc poll interval below five minutes (got %s)", interval)
	}
	urls := append([]string{}, defaultURLs...)
	if sourceConfig.Broker != "" {
		urls = []string{sourceConfig.Broker}
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "spc"))
	source := &Source{
		id: sourceConfig.ID, urls: urls, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "outlook.json"), TTL: interval},
		client: &http.Client{Timeout: 30 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, rawURL, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.spc" }

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxAreas)
	for day, rawURL := range s.urls {
		body, err := s.fetch(ctx, rawURL)
		if err != nil {
			if day == 0 {
				if cached, _, ok := s.cache.Load(); ok {
					body = cached
				} else {
					s.logger.Warn("spc fetch failed", "url", rawURL, "error", err)
					continue
				}
			} else {
				s.logger.Warn("spc fetch failed", "url", rawURL, "error", err)
				continue
			}
		} else if day == 0 {
			_ = s.cache.Store(body)
		}
		collection, err := geoutil.ParseFeatureCollection(body)
		if err != nil {
			continue
		}
		label := fmt.Sprintf("day%d", day+1)
		for i, feature := range collection.Features {
			polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
			if len(polys) == 0 {
				continue
			}
			name := geoutil.PropertyString(feature.Properties, "LABEL2", "LABEL")
			id := fmt.Sprintf("%s-%s-%d", label, geoutil.PropertyString(feature.Properties, "LABEL", "DN"), i)
			payload, err := json.Marshal(map[string]any{
				"kind": "outlook", "outlookId": id, "label": name, "day": label,
				"expires":  geoutil.PropertyString(feature.Properties, "EXPIRE_ISO", "EXPIRE"),
				"polygons": polys, "provider": "spc",
				"attribution": "NOAA Storm Prediction Center (public domain)",
			})
			if err != nil {
				continue
			}
			records = append(records, plugins.RawRecord{
				SourcePluginID: "spc", SourceInstanceID: s.id, OriginalID: "spc-" + id,
				Domain: "weather", ObservedUTC: now, Payload: payload,
			})
			if len(records) >= maxAreas {
				return records, nil
			}
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("spc sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("spc snapshot", "source", s.id, "areas", len(records))
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
