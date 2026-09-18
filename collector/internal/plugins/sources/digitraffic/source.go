// Package digitraffic polls Finnish Digitraffic marine ATON faults as
// Points and, when present, winter-navigation dirways as Paths.
// Sends Accept-Encoding: gzip. Licensed CC BY 4.0. AIS locations are
// not fetched.
package digitraffic

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
	defaultFaults  = "https://meri.digitraffic.fi/api/aton/v1/faults"
	defaultDirways = "https://meri.digitraffic.fi/api/winter-navigation/v2/dirways"
	pollDefault    = 10 * time.Minute
	pollFloor      = 2 * time.Minute
	maxFaults      = 80
	maxDirways     = 12
	minVertexKm    = 4.0
	maxVertices    = 48
)

type Source struct {
	id           string
	faultsURL    string
	dirwaysURL   string
	interval     time.Duration
	cache        pollutil.FileCache
	client       *http.Client
	logger       *slog.Logger
	fetchFaults  func(ctx context.Context) ([]byte, error)
	fetchDirways func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "maritime.digitraffic" {
		return nil, fmt.Errorf("unsupported digitraffic source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("digitraffic poll interval below two minutes (got %s)", interval)
	}
	faultsURL := defaultFaults
	dirwaysURL := defaultDirways
	if sourceConfig.Broker != "" {
		faultsURL = sourceConfig.Broker
	}
	if sourceConfig.Topic != "" {
		dirwaysURL = sourceConfig.Topic
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "digitraffic"))
	source := &Source{
		id: sourceConfig.ID, faultsURL: faultsURL, dirwaysURL: dirwaysURL, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "faults.json"), TTL: interval},
		client: &http.Client{Timeout: 40 * time.Second}, logger: logger,
	}
	headers := map[string]string{"Accept": "application/geo+json, application/json"}
	source.fetchFaults = func(ctx context.Context) ([]byte, error) {
		return pollutil.GetGzip(ctx, source.client, source.faultsURL, headers)
	}
	source.fetchDirways = func(ctx context.Context) ([]byte, error) {
		return pollutil.GetGzip(ctx, source.client, source.dirwaysURL, headers)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "maritime.digitraffic" }

func (s *Source) loadFaults(ctx context.Context) ([]byte, error) {
	body, err := s.fetchFaults(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("digitraffic using disk cache", "error", err)
		return cached, nil
	}
	return nil, err
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.loadFaults(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := geoutil.ParseFeatureCollection(body)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxFaults+maxDirways)
	for _, feature := range collection.Features {
		state := strings.ToLower(geoutil.PropertyString(feature.Properties, "state"))
		if state != "" && state != "open" {
			continue
		}
		lon, lat, ok := geoutil.Point(feature.Geometry)
		if !ok {
			continue
		}
		id := geoutil.FeatureID(feature, "id", "aton_id")
		name := geoutil.PropertyString(feature.Properties, "aton_name_en", "aton_name_fi", "aton_name_sv")
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "fault", "faultId": id, "name": name,
			"faultType": geoutil.PropertyString(feature.Properties, "type"),
			"atonType":  geoutil.PropertyString(feature.Properties, "aton_type"),
			"state":     geoutil.PropertyString(feature.Properties, "state"),
			"latitude":  lat, "longitude": lon,
			"attribution": "Digitraffic / Fintraffic (CC BY 4.0)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "digitraffic", SourceInstanceID: s.id, OriginalID: "digitraffic-fault-" + id,
			Domain: "maritime", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxFaults {
			break
		}
	}
	dirBody, err := s.fetchDirways(ctx)
	if err != nil {
		s.logger.Warn("digitraffic dirways skipped", "error", err)
		return records, nil
	}
	dirCollection, err := geoutil.ParseFeatureCollection(dirBody)
	if err != nil || len(dirCollection.Features) == 0 {
		return records, nil
	}
	added := 0
	for _, feature := range dirCollection.Features {
		lines := geoutil.DecimateLines(geoutil.Lines(feature.Geometry), minVertexKm, maxVertices)
		if len(lines) == 0 {
			continue
		}
		id := geoutil.FeatureID(feature, "id", "name")
		name := geoutil.PropertyString(feature.Properties, "name", "nameFi", "name_fi")
		if id == "" {
			id = name
		}
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "dirway", "dirwayId": id, "name": name, "segments": lines,
			"attribution": "Digitraffic / Fintraffic (CC BY 4.0)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "digitraffic", SourceInstanceID: s.id, OriginalID: "digitraffic-dirway-" + id,
			Domain: "maritime", ObservedUTC: now, Payload: payload,
		})
		added++
		if added >= maxDirways {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("digitraffic sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("digitraffic snapshot", "source", s.id, "records", len(records))
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
