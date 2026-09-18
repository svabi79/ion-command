// Package metalerts polls MET Norway MetAlerts 2.0 current GeoJSON and
// emits weather.alert Areas. Identifying User-Agent is required (pollutil
// already sends one). Credit MET Norway (NLOD 2.0 / CC BY 4.0).
package metalerts

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
	defaultURL  = "https://api.met.no/weatherapi/metalerts/2.0/current.json"
	pollDefault = 10 * time.Minute
	pollFloor   = 5 * time.Minute
	maxAlerts   = 24
	minVertexKm = 20.0
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
	if sourceConfig.Type != "weather.metalerts" {
		return nil, fmt.Errorf("unsupported metalerts source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("metalerts poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "metalerts"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "current.json"), TTL: interval},
		client: &http.Client{Timeout: 40 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.url, map[string]string{"Accept": "application/json"})
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.metalerts" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("metalerts using disk cache", "error", err)
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
			"event":       firstNonEmpty(geoutil.PropertyString(feature.Properties, "eventAwarenessName", "event", "title")),
			"severity":    geoutil.PropertyString(feature.Properties, "severity"),
			"headline":    geoutil.PropertyString(feature.Properties, "title"),
			"area":        geoutil.PropertyString(feature.Properties, "area"),
			"expires":     geoutil.PropertyString(feature.Properties, "expires", "onset"),
			"polygons":    polys,
			"provider":    "metnorway",
			"attribution": "MET Norway (NLOD 2.0 / CC BY 4.0)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "metalerts", SourceInstanceID: s.id, OriginalID: "metalerts-" + id,
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxAlerts {
			break
		}
	}
	return records, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("metalerts sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("metalerts snapshot", "source", s.id, "alerts", len(records))
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
