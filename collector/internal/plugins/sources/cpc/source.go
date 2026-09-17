// Package cpc polls NOAA Climate Prediction Center 8–14 day temperature
// and precipitation outlook polygons. Public US Government work. "Normal"
// / equal-chance categories are skipped so CONUS is not a solid fill.
package cpc

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
	pollDefault = 24 * time.Hour
	pollFloor   = 6 * time.Hour
	maxAreas    = 16
	minVertexKm = 40.0
	maxVertices = 32
)

var defaultURLs = []string{
	"https://mapservices.weather.noaa.gov/vector/rest/services/outlooks/cpc_8_14_day_outlk/MapServer/0/query?where=cat%20%3C%3E%20%27Normal%27&outFields=objectid,cat,prob,start_date,end_date&f=geojson&outSR=4326&maxAllowableOffset=0.2",
	"https://mapservices.weather.noaa.gov/vector/rest/services/outlooks/cpc_8_14_day_outlk/MapServer/1/query?where=cat%20%3C%3E%20%27Normal%27&outFields=objectid,cat,prob,start_date,end_date&f=geojson&outSR=4326&maxAllowableOffset=0.2",
}

var layerNames = []string{"temperature", "precipitation"}

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
	if sourceConfig.Type != "weather.cpc" {
		return nil, fmt.Errorf("unsupported cpc source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("cpc poll interval below six hours (got %s)", interval)
	}
	urls := append([]string{}, defaultURLs...)
	if sourceConfig.Broker != "" {
		urls = []string{sourceConfig.Broker}
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "cpc"))
	source := &Source{
		id: sourceConfig.ID, urls: urls, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "outlook.json"), TTL: interval},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, rawURL, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.cpc" }

func skipCategory(cat string) bool {
	lower := strings.ToLower(strings.TrimSpace(cat))
	return lower == "" || lower == "normal" || lower == "ec" || lower == "near normal" || lower == "equal chances"
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxAreas)
	var lastBody []byte
	for i, rawURL := range s.urls {
		body, err := s.fetch(ctx, rawURL)
		if err != nil {
			if cached, ok := s.cache.LoadStale(); ok && len(records) == 0 {
				s.logger.Warn("cpc using disk cache", "error", err)
				body = cached
			} else {
				s.logger.Warn("cpc layer failed", "url", rawURL, "error", err)
				continue
			}
		} else {
			lastBody = body
		}
		collection, err := geoutil.ParseFeatureCollection(body)
		if err != nil {
			continue
		}
		variable := "temperature"
		if i < len(layerNames) {
			variable = layerNames[i]
		}
		if len(s.urls) == 1 {
			variable = "outlook"
		}
		for _, feature := range collection.Features {
			cat := geoutil.PropertyString(feature.Properties, "cat", "CAT")
			if skipCategory(cat) {
				continue
			}
			polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
			if len(polys) == 0 {
				continue
			}
			id := geoutil.FeatureID(feature, "objectid", "OBJECTID")
			if id == "" {
				id = fmt.Sprintf("%s-%s-%d", variable, cat, len(records))
			}
			label := strings.TrimSpace(cat + " " + variable)
			payload, err := json.Marshal(map[string]any{
				"kind": "outlook", "outlookId": variable + "-" + id, "label": label,
				"day": "8-14", "polygons": polys, "provider": "cpc",
				"attribution": "NOAA Climate Prediction Center 8–14 day outlook",
			})
			if err != nil {
				continue
			}
			records = append(records, plugins.RawRecord{
				SourcePluginID: "cpc", SourceInstanceID: s.id, OriginalID: "cpc-" + variable + "-" + id,
				Domain: "weather", ObservedUTC: now, Payload: payload,
			})
			if len(records) >= maxAreas {
				if len(lastBody) > 0 {
					_ = s.cache.Store(lastBody)
				}
				return records, nil
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
			s.logger.Warn("cpc sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("cpc snapshot", "source", s.id, "areas", len(records))
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
