// Package sua polls FAA Special Use Airspace polygons from the public
// ArcGIS FeatureServer. Public US Government work. Coarsened and capped.
package sua

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
	defaultURL  = "https://services6.arcgis.com/ssFJjBXIUyZDrSYZ/arcgis/rest/services/Special_Use_Airspace/FeatureServer/0/query?where=TYPE_CODE%20IN%20(%27P%27%2C%27R%27%2C%27W%27%2C%27MOA%27%2C%27A%27)&outFields=NAME,TYPE_CODE,TIMESOFUSE,CITY,STATE&f=geojson&outSR=4326&resultRecordCount=80&maxAllowableOffset=0.08"
	pollDefault = 24 * time.Hour
	pollFloor   = 6 * time.Hour
	maxAreas    = 40
	minVertexKm = 20.0
	maxVertices = 28
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
	if sourceConfig.Type != "aviation.sua" {
		return nil, fmt.Errorf("unsupported sua source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("sua poll interval below six hours (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "sua"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "sua.json"), TTL: interval},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.sua" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("sua using disk cache", "error", err)
			return cached, nil
		}
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

func typeRank(code string) int {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "P":
		return 0
	case "R":
		return 1
	case "W":
		return 2
	case "MOA":
		return 3
	default:
		return 4
	}
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
	type ranked struct {
		id, title, class, times string
		rank                    int
		polys                   [][][][]float64
	}
	items := make([]ranked, 0, len(collection.Features))
	for _, feature := range collection.Features {
		polys := geoutil.DecimatePolygons(geoutil.Polygons(feature.Geometry), minVertexKm, maxVertices)
		if len(polys) == 0 {
			continue
		}
		id := geoutil.FeatureID(feature, "NAME", "OBJECTID")
		if id == "" {
			continue
		}
		code := geoutil.PropertyString(feature.Properties, "TYPE_CODE")
		items = append(items, ranked{
			id: id, title: geoutil.PropertyString(feature.Properties, "NAME"),
			class: code, times: geoutil.PropertyString(feature.Properties, "TIMESOFUSE", "CITY", "STATE"),
			rank: typeRank(code), polys: polys,
		})
	}
	for i := 0; i < len(items); i++ {
		best := i
		for j := i + 1; j < len(items); j++ {
			if items[j].rank < items[best].rank {
				best = j
			}
		}
		items[i], items[best] = items[best], items[i]
	}
	now := time.Now().UTC()
	if len(items) > maxAreas {
		items = items[:maxAreas]
	}
	records := make([]plugins.RawRecord, 0, len(items))
	for _, item := range items {
		payload, err := json.Marshal(map[string]any{
			"kind": "airspace", "spaceId": item.id, "title": item.title,
			"class": item.class, "hazard": item.times, "polygons": item.polys,
			"provider": "faa-sua", "attribution": "FAA Special Use Airspace (public domain)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "sua", SourceInstanceID: s.id, OriginalID: "sua-" + item.id,
			Domain: "aviation", ObservedUTC: now, Payload: payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("sua sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("sua snapshot", "source", s.id, "areas", len(records))
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
