// Package satnogs polls SatNOGS Network ground stations and emits Online
// stations as points. CC BY-SA data; hobby-scale daily cache.
package satnogs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://network.satnogs.org/api/stations/?format=json"
	pollDefault = 6 * time.Hour
	pollFloor   = time.Hour
	maxStations = 400
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
	if sourceConfig.Type != "orbital.satnogs" {
		return nil, fmt.Errorf("unsupported satnogs source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("satnogs poll interval below one hour (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "satnogs"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "stations.json"), TTL: interval},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "orbital.satnogs" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("satnogs using disk cache", "error", err)
			return cached, nil
		}
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

type station struct {
	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Status string  `json:"status"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	var stations []station
	if err := json.Unmarshal(body, &stations); err != nil {
		return nil, fmt.Errorf("decode satnogs stations: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxStations)
	for _, st := range stations {
		if st.Status != "Online" || st.ID == 0 {
			continue
		}
		id := strconv.Itoa(st.ID)
		payload, err := json.Marshal(map[string]any{
			"kind": "groundstation", "stationId": id, "name": st.Name,
			"status": st.Status, "latitude": st.Lat, "longitude": st.Lng,
			"attribution": "SatNOGS Network",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "satnogs", SourceInstanceID: s.id, OriginalID: "satnogs-" + id,
			Domain: "orbital", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxStations {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("satnogs sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("satnogs snapshot", "source", s.id, "stations", len(records))
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
