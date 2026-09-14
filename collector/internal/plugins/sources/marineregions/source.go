// Package marineregions polls Marine Regions EEZ boundary lines
// (200 NM, CC BY 4.0) and emits coarsened LineStrings for the globe.
// Full-resolution polygons are not fetched; WFS is paged and decimated
// (min 40 km, max 48 vertices, cap 220).
package marineregions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/geoutil"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	wfsBase     = "https://geo.vliz.be/geoserver/MarineRegions/wfs"
	pollDefault = 168 * time.Hour
	pollFloor   = 24 * time.Hour
	pageSize    = 40
	maxPages    = 8
	maxLines    = 220
	minVertexKm = 40.0
	maxVertices = 48
	attribution = "Marineregions.org / Flanders Marine Institute (CC BY 4.0)"
)

type Source struct {
	id       string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context, start int) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "geography.marineregions" {
		return nil, fmt.Errorf("unsupported marineregions source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("marineregions poll interval below one day (got %s)", interval)
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "marineregions"))
	source := &Source{
		id: sourceConfig.ID, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "eez.json"), TTL: interval},
		client: &http.Client{Timeout: 90 * time.Second}, logger: logger,
	}
	base := wfsBase
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	source.fetch = func(ctx context.Context, start int) ([]byte, error) {
		query := url.Values{}
		query.Set("service", "WFS")
		query.Set("version", "1.1.0")
		query.Set("request", "GetFeature")
		query.Set("typeName", "MarineRegions:eez_boundaries")
		query.Set("outputFormat", "application/json")
		query.Set("maxFeatures", fmt.Sprintf("%d", pageSize))
		query.Set("startIndex", fmt.Sprintf("%d", start))
		query.Set("cql_filter", "line_type='200 NM'")
		return pollutil.Get(ctx, source.client, base+"?"+query.Encode(), nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.marineregions" }

type simplified struct {
	Lines []simplifiedLine `json:"lines"`
}

type simplifiedLine struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Segments [][][]float64 `json:"segments"`
}

func (s *Source) refresh(ctx context.Context) (simplified, error) {
	if cached, _, ok := s.cache.Load(); ok {
		var ready simplified
		if json.Unmarshal(cached, &ready) == nil && len(ready.Lines) > 0 {
			return ready, nil
		}
	}
	out := simplified{}
	for page := 0; page < maxPages && len(out.Lines) < maxLines; page++ {
		body, err := s.fetch(ctx, page*pageSize)
		if err != nil {
			if len(out.Lines) == 0 {
				if cached, ok := s.cache.LoadStale(); ok {
					var ready simplified
					if json.Unmarshal(cached, &ready) == nil && len(ready.Lines) > 0 {
						s.logger.Warn("marineregions using disk cache", "error", err)
						return ready, nil
					}
				}
				return simplified{}, err
			}
			s.logger.Warn("marineregions page failed", "page", page, "error", err)
			break
		}
		collection, err := geoutil.ParseFeatureCollection(body)
		if err != nil || len(collection.Features) == 0 {
			break
		}
		for _, feature := range collection.Features {
			lines := geoutil.DecimateLines(geoutil.Lines(feature.Geometry), minVertexKm, maxVertices)
			if len(lines) == 0 {
				continue
			}
			id := geoutil.FeatureID(feature, "line_id", "mrgid_eez1")
			if id == "" {
				id = fmt.Sprintf("eez-%d", len(out.Lines))
			}
			name := geoutil.PropertyString(feature.Properties, "line_name", "eez1", "territory1", "sovereign1")
			out.Lines = append(out.Lines, simplifiedLine{ID: id, Name: name, Segments: lines})
			if len(out.Lines) >= maxLines {
				break
			}
		}
		if len(collection.Features) < pageSize {
			break
		}
	}
	if encoded, err := json.Marshal(out); err == nil {
		_ = s.cache.Store(encoded)
	}
	return out, nil
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	data, err := s.refresh(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(data.Lines))
	for _, line := range data.Lines {
		payload, err := json.Marshal(map[string]any{
			"kind": "eez", "eezId": line.ID, "name": line.Name, "segments": line.Segments,
			"attribution": attribution,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "marineregions", SourceInstanceID: s.id, OriginalID: "eez-" + line.ID,
			Domain: "geography", ObservedUTC: now, Payload: payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("marineregions sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("marineregions snapshot", "source", s.id, "lines", len(records))
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
