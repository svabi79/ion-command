// Package bcwildfire polls current BC Wildfire perimeters from the public
// ArcGIS MapServer as coarsened Areas. Licensed OGL-BC. The points layer
// is not fetched.
package bcwildfire

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
	defaultURL  = "https://delivery.maps.gov.bc.ca/arcgis/rest/services/mpcm/bcgwpub/MapServer/624/query?where=1%3D1&outFields=FIRE_NUMBER,FIRE_STATUS,FIRE_SIZE_HECTARES,GEOGRAPHIC_DESCRIPTION,FIRE_OF_NOTE_NAME&returnGeometry=true&f=geojson&outSR=4326&resultRecordCount=24&orderByFields=FIRE_SIZE_HECTARES%20DESC&maxAllowableOffset=0.02&geometry=-139.5,48.0,-113.5,60.5&geometryType=esriGeometryEnvelope&inSR=4326&spatialRel=esriSpatialRelIntersects"
	pollDefault = 15 * time.Minute
	pollFloor   = 5 * time.Minute
	maxAreas    = 24
	minVertexKm = 8.0
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
	if sourceConfig.Type != "wildfire.bc" {
		return nil, fmt.Errorf("unsupported bc wildfire source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("bc wildfire poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "bcwildfire"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "perimeters.json"), TTL: interval},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "wildfire.bc" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("bcwildfire using disk cache", "error", err)
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
	records := make([]plugins.RawRecord, 0, maxAreas)
	for _, feature := range collection.Features {
		polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
		if len(polys) == 0 {
			continue
		}
		id := geoutil.FeatureID(feature, "FIRE_NUMBER", "OBJECTID")
		name := geoutil.PropertyString(feature.Properties, "FIRE_OF_NOTE_NAME", "GEOGRAPHIC_DESCRIPTION", "FIRE_NUMBER")
		if id == "" {
			id = name
		}
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "perimeter", "perimeterId": "bc-" + id, "name": name,
			"category":    geoutil.PropertyString(feature.Properties, "FIRE_STATUS"),
			"polygons":    polys,
			"attribution": "BC Wildfire Service (OGL-BC)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "bcwildfire", SourceInstanceID: s.id, OriginalID: "bcwildfire-" + id,
			Domain: "wildfire", ObservedUTC: now, Payload: payload,
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
			s.logger.Warn("bcwildfire sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("bcwildfire snapshot", "source", s.id, "areas", len(records))
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
