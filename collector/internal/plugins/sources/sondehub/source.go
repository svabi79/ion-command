// Package sondehub polls live radiosonde positions from SondeHub v2.
// Licensed CC BY-SA 2.0; attribute SondeHub. Fixtures stay tiny.
package sondehub

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://api.v2.sondehub.org/sondes"
	pollDefault = 2 * time.Minute
	pollFloor   = 1 * time.Minute
	maxSondes   = 80
	maxAge      = 6 * time.Hour
)

type Source struct {
	id       string
	url      string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	now      func() time.Time
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.sondehub" {
		return nil, fmt.Errorf("unsupported sondehub source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("sondehub poll interval below one minute (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "sondehub"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "sondes.json"), TTL: interval},
		client: &http.Client{Timeout: 40 * time.Second}, logger: logger, now: func() time.Time { return time.Now().UTC() },
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.sondehub" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("sondehub using disk cache", "error", err)
		return cached, nil
	}
	return nil, err
}

type sondeRecord struct {
	Serial   string  `json:"serial"`
	Type     string  `json:"type"`
	Subtype  string  `json:"subtype"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	Alt      float64 `json:"alt"`
	Datetime string  `json:"datetime"`
	Received string  `json:"time_received"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	var doc map[string]sondeRecord
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode sondehub: %w", err)
	}
	now := s.now()
	type ranked struct {
		rec  sondeRecord
		when time.Time
	}
	keep := make([]ranked, 0, len(doc))
	for serial, rec := range doc {
		if rec.Serial == "" {
			rec.Serial = serial
		}
		if rec.Lat == 0 && rec.Lon == 0 {
			continue
		}
		when := parseSondeTime(rec.Received, rec.Datetime)
		if when.IsZero() {
			when = now
		}
		if now.Sub(when) > maxAge {
			continue
		}
		keep = append(keep, ranked{rec: rec, when: when})
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].when.After(keep[j].when) })
	if len(keep) > maxSondes {
		keep = keep[:maxSondes]
	}
	records := make([]plugins.RawRecord, 0, len(keep))
	for _, item := range keep {
		kind := item.rec.Subtype
		if kind == "" {
			kind = item.rec.Type
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "sonde", "serial": item.rec.Serial, "sondeType": kind,
			"latitude": item.rec.Lat, "longitude": item.rec.Lon, "altitudeM": item.rec.Alt,
			"attribution": "SondeHub (CC BY-SA 2.0)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "sondehub", SourceInstanceID: s.id, OriginalID: "sondehub-" + item.rec.Serial,
			Domain: "weather", ObservedUTC: item.when, Payload: payload,
		})
	}
	return records, nil
}

func parseSondeTime(values ...string) time.Time {
	for _, value := range values {
		if value == "" {
			continue
		}
		if when, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return when.UTC()
		}
		if when, err := time.Parse(time.RFC3339, value); err == nil {
			return when.UTC()
		}
	}
	return time.Time{}
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("sondehub sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("sondehub snapshot", "source", s.id, "sondes", len(records))
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
