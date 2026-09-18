// Package eccc polls Environment and Climate Change Canada weather-alert
// GeoJSON and emits weather.alert Areas. Attribute Government of Canada / MSC.
package eccc

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
	defaultURL  = "https://api.weather.gc.ca/collections/weather-alerts/items?f=json&limit=50"
	pollDefault = 10 * time.Minute
	pollFloor   = 5 * time.Minute
	maxAlerts   = 40
	maxPages    = 4
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
	fetch    func(ctx context.Context, rawURL string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.eccc" {
		return nil, fmt.Errorf("unsupported eccc source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("eccc poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "eccc"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "alerts.json"), TTL: interval},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, rawURL, map[string]string{"Accept": "application/json"})
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.eccc" }

type pageDoc struct {
	Links []struct {
		Rel  string `json:"rel"`
		Href string `json:"href"`
	} `json:"links"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxAlerts)
	rawURL := s.url
	var lastBody []byte
	for page := 0; page < maxPages && rawURL != "" && len(records) < maxAlerts; page++ {
		body, err := s.fetch(ctx, rawURL)
		if err != nil {
			if page == 0 {
				if cached, ok := s.cache.LoadStale(); ok {
					s.logger.Warn("eccc using disk cache", "error", err)
					body = cached
				} else {
					return nil, err
				}
			} else {
				break
			}
		} else {
			lastBody = body
		}
		var pagePayload pageDoc
		if err := json.Unmarshal(body, &pagePayload); err != nil {
			return nil, err
		}
		collection, err := geoutil.ParseFeatureCollection(body)
		if err != nil {
			return nil, err
		}
		for _, feature := range collection.Features {
			status := geoutil.PropertyString(feature.Properties, "status_en", "status")
			if status != "" && !strings.EqualFold(status, "Active") && !strings.EqualFold(status, "Actual") {
				continue
			}
			polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
			if len(polys) == 0 {
				continue
			}
			id := geoutil.FeatureID(feature, "alert_code", "feature_id", "id")
			if id == "" {
				continue
			}
			payload, err := json.Marshal(map[string]any{
				"kind":        "alert",
				"alertId":     id,
				"event":       geoutil.PropertyString(feature.Properties, "alert_name_en", "alert_short_name_en"),
				"severity":    geoutil.PropertyString(feature.Properties, "risk_colour_en", "confidence_en"),
				"headline":    geoutil.PropertyString(feature.Properties, "alert_name_en"),
				"area":        geoutil.PropertyString(feature.Properties, "feature_name_en", "province"),
				"expires":     geoutil.PropertyString(feature.Properties, "expiration_datetime", "event_end_datetime"),
				"polygons":    polys,
				"provider":    "eccc",
				"attribution": "Government of Canada / MSC",
			})
			if err != nil {
				continue
			}
			records = append(records, plugins.RawRecord{
				SourcePluginID: "eccc", SourceInstanceID: s.id, OriginalID: "eccc-" + id,
				Domain: "weather", ObservedUTC: now, Payload: payload,
			})
			if len(records) >= maxAlerts {
				break
			}
		}
		rawURL = ""
		for _, link := range pagePayload.Links {
			if strings.EqualFold(link.Rel, "next") && strings.TrimSpace(link.Href) != "" {
				rawURL = link.Href
				break
			}
		}
	}
	if len(lastBody) > 0 {
		_ = s.cache.Store(lastBody)
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("eccc sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("eccc snapshot", "source", s.id, "alerts", len(records))
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
