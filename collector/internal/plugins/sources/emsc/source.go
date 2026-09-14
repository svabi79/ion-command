// Package emsc polls the EMSC/EMSC-CSEM FDSN JSON event feed as a second
// seismic source alongside USGS. Public, no key. Websocket standing-order
// exists but HTTP FDSN is the same canonical pipeline as USGS.
package emsc

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
	defaultURL  = "https://www.seismicportal.eu/fdsnws/event/1/query?limit=80&format=json&minmag=2.5"
	pollDefault = 3 * time.Minute
	pollFloor   = time.Minute
	maxQuakes   = 80
)

type Source struct {
	id       string
	url      string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	seen     map[string]string
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "earthquake.emsc" {
		return nil, fmt.Errorf("unsupported emsc source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("emsc poll interval below one minute (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "emsc"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval, seen: make(map[string]string),
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "events.json"), TTL: interval},
		client: &http.Client{Timeout: 30 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "earthquake.emsc" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, _, ok := s.cache.Load(); ok {
		s.logger.Warn("emsc using disk cache", "error", err)
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
	if len(s.seen) > 4096 {
		s.seen = make(map[string]string)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxQuakes)
	for _, feature := range collection.Features {
		lon, lat, ok := geoutil.Point(feature.Geometry)
		if !ok {
			lon = geoutil.PropertyFloat(feature.Properties, "lon")
			lat = geoutil.PropertyFloat(feature.Properties, "lat")
			if lon == 0 && lat == 0 {
				continue
			}
			ok = true
		}
		_ = ok
		id := geoutil.PropertyString(feature.Properties, "unid", "source_id")
		if id == "" {
			id = geoutil.FeatureID(feature)
		}
		if id == "" {
			continue
		}
		updated := geoutil.PropertyString(feature.Properties, "lastupdate", "time")
		if previous, exists := s.seen[id]; exists && previous == updated {
			continue
		}
		s.seen[id] = updated
		mag := geoutil.PropertyFloat(feature.Properties, "mag")
		depth := geoutil.PropertyFloat(feature.Properties, "depth")
		observed := now
		if stamp := geoutil.PropertyString(feature.Properties, "time"); stamp != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
				observed = parsed.UTC()
			} else if parsed, err := time.Parse(time.RFC3339, stamp); err == nil {
				observed = parsed.UTC()
			}
		}
		payload, err := json.Marshal(map[string]any{
			"quakeId": id, "longitude": lon, "latitude": lat,
			"magnitude": mag, "depthKm": depth,
			"place": geoutil.PropertyString(feature.Properties, "flynn_region"),
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "emsc", SourceInstanceID: s.id, OriginalID: "emsc-" + id + "-" + updated,
			Domain: "geophysics", ObservedUTC: observed, Payload: payload,
		})
		if len(records) >= maxQuakes {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("emsc sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("emsc snapshot", "source", s.id, "quakes", len(records))
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
