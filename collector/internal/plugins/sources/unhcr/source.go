// Package unhcr polls UNHCR GIS persons-of-concern site points
// (ArcGIS FeatureServer GeoJSON). Attribution: UNHCR. Locations only;
// no population counts are invented from empty fields.
package unhcr

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
	defaultURL  = "https://gis.unhcr.org/arcgis/rest/services/core_v2/wrl_prp_p_unhcr_PoC/FeatureServer/0/query?where=1%3D1&outFields=iso3,gis_name,loc_type,poc_subtype,gis_status&f=geojson&resultRecordCount=400"
	pollDefault = 24 * time.Hour
	pollFloor   = 6 * time.Hour
	maxSites    = 400
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
	if sourceConfig.Type != "humanitarian.unhcr" {
		return nil, fmt.Errorf("unsupported unhcr source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("unhcr poll interval below six hours (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "unhcr"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "poc.json"), TTL: interval},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "humanitarian.unhcr" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("unhcr using disk cache", "error", err)
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
	records := make([]plugins.RawRecord, 0, maxSites)
	for _, feature := range collection.Features {
		status := strings.ToLower(geoutil.PropertyString(feature.Properties, "gis_status"))
		if status != "" && status != "active" && status != "open" {
			continue
		}
		lon, lat, ok := geoutil.Point(feature.Geometry)
		if !ok {
			continue
		}
		id := geoutil.FeatureID(feature, "pcode", "objectid", "globalid")
		name := geoutil.PropertyString(feature.Properties, "gis_name", "name_alt")
		if id == "" {
			id = name
		}
		if id == "" {
			continue
		}
		kind := geoutil.PropertyString(feature.Properties, "poc_subtype", "loc_type")
		payload, err := json.Marshal(map[string]any{
			"kind": "site", "siteId": id, "name": name, "siteKind": kind,
			"country":  geoutil.PropertyString(feature.Properties, "iso3"),
			"latitude": lat, "longitude": lon,
			"attribution": "UNHCR",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "unhcr", SourceInstanceID: s.id, OriginalID: "unhcr-" + id,
			Domain: "humanitarian", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxSites {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("unhcr sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("unhcr snapshot", "source", s.id, "sites", len(records))
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
