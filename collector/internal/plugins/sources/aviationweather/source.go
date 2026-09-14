// Package aviationweather polls AviationWeather.gov SIGMET (polygons) and
// G-AIRMET (closed rings only) GeoJSON. No key. Not a warning system.
package aviationweather

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
	defaultSigmet  = "https://aviationweather.gov/api/data/isigmet?format=geojson"
	defaultGAirmet = "https://aviationweather.gov/api/data/gairmet?format=geojson"
	pollDefault    = 5 * time.Minute
	pollFloor      = 2 * time.Minute
	maxSIGMET      = 50
	maxGAirmet     = 20
	minVertexKm    = 20.0
	maxVertices    = 32
)

type Source struct {
	id          string
	sigmetURL   string
	gairmetURL  string
	interval    time.Duration
	cache       pollutil.FileCache
	client      *http.Client
	logger      *slog.Logger
	fetchSigmet func(ctx context.Context) ([]byte, error)
	fetchAirmet func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "aviation.aviationweather" {
		return nil, fmt.Errorf("unsupported aviationweather source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("aviationweather poll interval below two minutes (got %s)", interval)
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "aviationweather"))
	source := &Source{
		id: sourceConfig.ID, sigmetURL: defaultSigmet, gairmetURL: defaultGAirmet, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "hazards.json"), TTL: interval},
		client: &http.Client{Timeout: 40 * time.Second}, logger: logger,
	}
	if sourceConfig.Broker != "" {
		source.sigmetURL = sourceConfig.Broker
	}
	if sourceConfig.Topic != "" {
		source.gairmetURL = sourceConfig.Topic
	}
	source.fetchSigmet = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.sigmetURL, nil)
	}
	source.fetchAirmet = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.gairmetURL, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.aviationweather" }

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxSIGMET+maxGAirmet)
	if body, err := s.fetchSigmet(ctx); err == nil {
		records = append(records, s.decode(body, now, "sigmet", maxSIGMET)...)
		_ = s.cache.Store(body)
	} else if cached, _, ok := s.cache.Load(); ok {
		s.logger.Warn("aviationweather sigmet using cache", "error", err)
		records = append(records, s.decode(cached, now, "sigmet", maxSIGMET)...)
	} else {
		s.logger.Warn("aviationweather sigmet failed", "error", err)
	}
	if body, err := s.fetchAirmet(ctx); err == nil {
		records = append(records, s.decode(body, now, "airmet", maxGAirmet)...)
	} else {
		s.logger.Warn("aviationweather gairmet failed", "error", err)
	}
	return records, nil
}

func (s *Source) decode(body []byte, now time.Time, kind string, limit int) []plugins.RawRecord {
	collection, err := geoutil.ParseFeatureCollection(body)
	if err != nil {
		return nil
	}
	records := make([]plugins.RawRecord, 0, limit)
	for i, feature := range collection.Features {
		polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
		if len(polys) == 0 {
			continue
		}
		id := geoutil.FeatureID(feature, "icaoId", "seriesId", "product", "tag")
		if id == "" {
			id = fmt.Sprintf("%s-%d", kind, i)
		}
		hazard := geoutil.PropertyString(feature.Properties, "hazard", "qualifier", "product")
		title := geoutil.PropertyString(feature.Properties, "firName", "icaoId", "product")
		if title == "" {
			title = hazard
		}
		validUntil := geoutil.PropertyString(feature.Properties, "validTimeTo")
		if validUntil == "" {
			if unix := geoutil.PropertyFloat(feature.Properties, "validTime"); unix > 0 {
				validUntil = time.Unix(int64(unix), 0).UTC().Format(time.RFC3339)
			}
		}
		payload, err := json.Marshal(map[string]any{
			"kind": kind, "spaceId": kind + "-" + id, "title": title, "hazard": hazard,
			"validUntil": validUntil, "polygons": polys, "provider": "aviationweather",
			"attribution": "AviationWeather.gov / NOAA (public domain)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "aviationweather", SourceInstanceID: s.id, OriginalID: kind + "-" + id,
			Domain: "aviation", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= limit {
			break
		}
	}
	return records
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("aviationweather sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("aviationweather snapshot", "source", s.id, "areas", len(records))
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
