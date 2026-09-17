// Package fews polls FEWS NET IPC phase-map polygons from the FDW GeoJSON
// API. Crisis and worse (IPC 3+) only, coarsened and capped. The plugin is
// registered disabled: IPC maps mix FEWS NET public records with IPC
// partnership copyright, and the live payload is tens of megabytes.
package fews

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
	defaultURL  = "https://fdw.fews.net/api/ipcphasemap/?format=geojson"
	pollDefault = 168 * time.Hour
	pollFloor   = 24 * time.Hour
	maxBytes    = 32 << 20
	maxAreas    = 24
	minPhase    = 3
	minVertexKm = 40.0
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
	if sourceConfig.Type != "humanitarian.fews" {
		return nil, fmt.Errorf("unsupported fews source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("fews poll interval below one day (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "fews"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "ipcphasemap.geojson"), TTL: interval},
		client: &http.Client{Timeout: 90 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) {
		return pollutil.GetLimit(ctx, source.client, source.url, nil, maxBytes)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "humanitarian.fews" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("fews using disk cache", "error", err)
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
		phase := int(geoutil.PropertyFloat(feature.Properties, "value", "ipc_phase", "phase"))
		if phase < minPhase {
			continue
		}
		polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
		if len(polys) == 0 {
			continue
		}
		id := geoutil.FeatureID(feature, "fnid", "id")
		if id == "" {
			id = fmt.Sprintf("%s-%d", geoutil.PropertyString(feature.Properties, "country_code"), phase)
		}
		label := fmt.Sprintf("IPC %d", phase)
		payload, err := json.Marshal(map[string]any{
			"kind": "foodsecurity", "areaId": id, "label": label,
			"phase":       fmt.Sprintf("%d", phase),
			"country":     geoutil.PropertyString(feature.Properties, "country_code", "country"),
			"polygons":    polys,
			"attribution": "FEWS NET / IPC (FDW ipcphasemap); mixed copyright — operator-enabled only",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "fews", SourceInstanceID: s.id, OriginalID: "fews-" + id,
			Domain: "humanitarian", ObservedUTC: now, Payload: payload,
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
			s.logger.Warn("fews sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("fews snapshot", "source", s.id, "areas", len(records))
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
