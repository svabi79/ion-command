// Package milbases polls NTAD/BTS military installation polygons.
// US Government work; coarsened and capped for globe display.
package milbases

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
	defaultURL  = "https://services.arcgis.com/xOi1kZaI0eWDREZv/arcgis/rest/services/NTAD_Military_Bases/FeatureServer/0/query?where=siteOperationalStatus%3D%27act%27&outFields=featureName,siteName,siteReportingComponent,siteOperationalStatus,stateNameCode,countryName,OBJECTID&orderByFields=Shape__Area%20DESC&f=geojson&outSR=4326&resultRecordCount=50&maxAllowableOffset=0.04"
	pollDefault = 168 * time.Hour
	pollFloor   = 24 * time.Hour
	maxAreas    = 40
	minVertexKm = 6.0
	maxVertices = 24
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
	if sourceConfig.Type != "geography.milbases" {
		return nil, fmt.Errorf("unsupported milbases source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("milbases poll interval below one day (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "milbases"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "bases.json"), TTL: interval},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.milbases" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("milbases using disk cache", "error", err)
			return cached, nil
		}
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
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
	records := make([]plugins.RawRecord, 0, maxAreas)
	for _, feature := range collection.Features {
		polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
		if len(polys) == 0 {
			continue
		}
		id := geoutil.FeatureID(feature, "OBJECTID", "siteName", "featureName")
		name := geoutil.PropertyString(feature.Properties, "featureName", "siteName")
		if id == "" {
			id = name
		}
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "base", "baseId": id, "name": name,
			"component":   geoutil.PropertyString(feature.Properties, "siteReportingComponent"),
			"status":      geoutil.PropertyString(feature.Properties, "siteOperationalStatus"),
			"state":       strings.ToUpper(geoutil.PropertyString(feature.Properties, "stateNameCode")),
			"country":     geoutil.PropertyString(feature.Properties, "countryName"),
			"polygons":    polys,
			"attribution": "USDOT BTS National Transportation Atlas Database, Military Bases (US Government work)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "milbases", SourceInstanceID: s.id, OriginalID: "base-" + id,
			Domain: "geography", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxAreas {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("milbases sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("milbases snapshot", "source", s.id, "areas", len(records))
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
