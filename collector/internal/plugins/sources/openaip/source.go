// Package openaip polls OpenAIP airspace polygons. CC BY-NC; API key
// required (x-openaip-api-key). Fail-closed without a key. Geometry is
// coarsened; FIR/UIR/TMA/CTR preferred, cap 40 areas.
package openaip

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
	defaultURL  = "https://api.core.openaip.net/api/airspaces?limit=100"
	pollDefault = 24 * time.Hour
	pollFloor   = 6 * time.Hour
	maxSpaces   = 40
	minVertexKm = 20.0
	maxVertices = 32
)

type Source struct {
	id       string
	url      string
	apiKey   string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "aviation.openaip" {
		return nil, fmt.Errorf("unsupported openaip source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(sourceConfig.ApiKey) == "" {
		return nil, fmt.Errorf("aviation.openaip requires apiKey (OpenAIP key in local.json)")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("openaip poll interval below six hours (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "openaip"))
	source := &Source{
		id: sourceConfig.ID, url: url, apiKey: sourceConfig.ApiKey, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "airspaces.json"), TTL: interval},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.url, map[string]string{"x-openaip-api-key": source.apiKey, "Accept": "application/json"})
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.openaip" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("openaip using disk cache", "error", err)
			return cached, nil
		}
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

type openaipItem struct {
	ID        string           `json:"_id"`
	Name      string           `json:"name"`
	Type      int              `json:"type"`
	IcaoClass int              `json:"icaoClass"`
	Country   string           `json:"country"`
	Geometry  geoutil.Geometry `json:"geometry"`
}

type openaipResponse struct {
	Items    []openaipItem     `json:"items"`
	Features []geoutil.Feature `json:"features"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxSpaces)
	var wrapped openaipResponse
	_ = json.Unmarshal(body, &wrapped)
	features := wrapped.Features
	if len(features) == 0 && len(wrapped.Items) > 0 {
		for _, item := range wrapped.Items {
			features = append(features, geoutil.Feature{
				ID:         item.ID,
				Properties: map[string]any{"name": item.Name, "type": item.Type, "icaoClass": item.IcaoClass, "country": item.Country},
				Geometry:   item.Geometry,
			})
		}
	}
	if len(features) == 0 {
		collection, err := geoutil.ParseFeatureCollection(body)
		if err == nil {
			features = collection.Features
		}
	}
	for _, feature := range features {
		polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
		if len(polys) == 0 {
			continue
		}
		id := geoutil.FeatureID(feature, "_id", "id", "name")
		if id == "" {
			continue
		}
		class := geoutil.PropertyString(feature.Properties, "icaoClass", "type", "country")
		payload, err := json.Marshal(map[string]any{
			"kind": "airspace", "spaceId": id,
			"title": geoutil.PropertyString(feature.Properties, "name"),
			"class": class, "polygons": polys, "provider": "openaip",
			"attribution": "OpenAIP (CC BY-NC)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "openaip", SourceInstanceID: s.id, OriginalID: "openaip-" + id,
			Domain: "aviation", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxSpaces {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("openaip sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("openaip snapshot", "source", s.id, "airspaces", len(records))
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
