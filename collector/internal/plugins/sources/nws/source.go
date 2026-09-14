// Package nws polls NWS active alerts (api.weather.gov) and emits one
// weather record per alert that carries native GeoJSON polygon geometry.
// Zone/county alerts without a polygon are skipped. Identifying User-Agent
// is required by NWS; pollutil already sends one. Public domain.
package nws

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
	"github.com/ion-command/ion-command/collector/internal/geoutil"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://api.weather.gov/alerts/active?status=actual"
	pollDefault = 5 * time.Minute
	pollFloor   = 2 * time.Minute
	maxAlerts   = 40
	minVertexKm = 15.0
	maxVertices = 32
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
	if sourceConfig.Type != "weather.nws" {
		return nil, fmt.Errorf("unsupported nws source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("nws poll interval below two minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "nws"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "alerts.json"), TTL: interval},
		client: &http.Client{Timeout: 40 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.url, map[string]string{"Accept": "application/geo+json"})
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.nws" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, _, ok := s.cache.Load(); ok {
		s.logger.Warn("nws using disk cache", "source", s.id, "error", err)
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
	records := make([]plugins.RawRecord, 0, maxAlerts)
	for _, feature := range collection.Features {
		eventName := geoutil.PropertyString(feature.Properties, "event")
		if strings.EqualFold(eventName, "Test Message") || strings.Contains(strings.ToUpper(geoutil.FeatureID(feature, "id")), "KEEPALIVE") {
			continue
		}
		status := geoutil.PropertyString(feature.Properties, "status")
		if status != "" && !strings.EqualFold(status, "Actual") {
			continue
		}
		polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
		if len(polys) == 0 {
			continue
		}
		id := geoutil.FeatureID(feature, "id")
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind":        "alert",
			"alertId":     id,
			"event":       eventName,
			"severity":    geoutil.PropertyString(feature.Properties, "severity"),
			"headline":    geoutil.PropertyString(feature.Properties, "headline"),
			"area":        geoutil.PropertyString(feature.Properties, "areaDesc"),
			"expires":     geoutil.PropertyString(feature.Properties, "expires", "ends"),
			"polygons":    polys,
			"provider":    "nws",
			"attribution": "NWS / NOAA (public domain)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "nws", SourceInstanceID: s.id, OriginalID: "nws-" + id,
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxAlerts {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("nws sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("nws snapshot", "source", s.id, "alerts", len(records))
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
