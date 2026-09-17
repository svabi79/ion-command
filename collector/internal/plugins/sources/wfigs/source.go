// Package wfigs polls NIFC/WFIGS current wildland-fire perimeters from the
// public ArcGIS FeatureServer. Coarsened and capped for globe display.
package wfigs

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
	// Current_WildlandFire_Perimeters is token-gated; WFIGS Interagency is the
	// public replacement with the same IRWIN perimeters.
	defaultURL  = "https://services3.arcgis.com/T4QMspbfLg3qTGWY/arcgis/rest/services/WFIGS_Interagency_Perimeters_Current/FeatureServer/0/query?where=attr_IncidentTypeCategory%3D%27WF%27&outFields=poly_IncidentName,attr_IncidentName,attr_IncidentTypeCategory,OBJECTID&f=geojson&outSR=4326&resultRecordCount=30&maxAllowableOffset=0.05"
	pollDefault = 15 * time.Minute
	pollFloor   = 5 * time.Minute
	maxAreas    = 30
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
	if sourceConfig.Type != "wildfire.wfigs" {
		return nil, fmt.Errorf("unsupported wfigs source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("wfigs poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "wfigs"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "perimeters.json"), TTL: interval},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "wildfire.wfigs" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("wfigs using disk cache", "error", err)
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
		id := geoutil.FeatureID(feature, "OBJECTID", "poly_IncidentName", "attr_IncidentName")
		name := geoutil.PropertyString(feature.Properties, "poly_IncidentName", "attr_IncidentName")
		if id == "" {
			id = name
		}
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "perimeter", "perimeterId": id, "name": name,
			"category": geoutil.PropertyString(feature.Properties, "attr_IncidentTypeCategory"),
			"polygons": polys, "attribution": "NIFC / WFIGS Interagency Perimeters",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "wfigs", SourceInstanceID: s.id, OriginalID: "wfigs-" + id,
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
			s.logger.Warn("wfigs sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("wfigs snapshot", "source", s.id, "areas", len(records))
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
