// Package gvp polls the Smithsonian Global Volcanism Program Holocene
// volcano WFS as GeoJSON points. Catalog, cached daily. CC scientific
// attribution: Smithsonian Institution / GVP.
package gvp

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
	defaultURL   = "https://webservices.volcano.si.edu/geoserver/GVP-VOTW/ows?service=WFS&version=1.0.0&request=GetFeature&typeName=GVP-VOTW:Smithsonian_VOTW_Holocene_Volcanoes&maxFeatures=400&outputFormat=application/json"
	pollDefault  = 24 * time.Hour
	pollFloor    = 6 * time.Hour
	maxVolcanoes = 400
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
	if sourceConfig.Type != "geophysics.gvp" {
		return nil, fmt.Errorf("unsupported gvp source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("gvp poll interval below six hours (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "gvp"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "volcanoes.json"), TTL: interval},
		client: &http.Client{Timeout: 90 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geophysics.gvp" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("gvp using disk cache", "error", err)
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
	records := make([]plugins.RawRecord, 0, maxVolcanoes)
	for _, feature := range collection.Features {
		lon, lat, ok := geoutil.Point(feature.Geometry)
		if !ok {
			continue
		}
		id := geoutil.PropertyString(feature.Properties, "Volcano_Number", "vnum", "VolcanoNumber", "Volc_Number")
		name := geoutil.PropertyString(feature.Properties, "Volcano_Name", "VolcanoName", "name")
		if id == "" {
			id = name
		}
		if id == "" {
			id = geoutil.FeatureID(feature)
		}
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "volcano", "volcanoId": "gvp-" + id, "name": name,
			"region":  geoutil.PropertyString(feature.Properties, "Region", "Subregion", "Country"),
			"catalog": true, "latitude": lat, "longitude": lon, "provider": "gvp",
			"attribution": "Smithsonian Institution Global Volcanism Program",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "gvp", SourceInstanceID: s.id, OriginalID: "gvp-" + id,
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
			s.logger.Warn("gvp sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("gvp snapshot", "source", s.id, "volcanoes", len(records))
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
