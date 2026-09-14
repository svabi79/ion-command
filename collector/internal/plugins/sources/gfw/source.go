// Package gfw polls Global Fishing Watch fishing-event points. Bearer
// token required, CC BY-NC, fail-closed. Coarse event Points only — no
// 4Wings raster/Field. SAR presence is a 4Wings raster product and is
// not fetched.
package gfw

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
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://gateway.api.globalfishingwatch.org/v3/events"
	pollDefault = 6 * time.Hour
	pollFloor   = time.Hour
	maxEvents   = 200
)

type Source struct {
	id       string
	url      string
	token    string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	now      func() time.Time
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "maritime.gfw" {
		return nil, fmt.Errorf("unsupported gfw source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(sourceConfig.ApiKey) == "" {
		return nil, fmt.Errorf("maritime.gfw requires apiKey (GFW bearer token in local.json)")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("gfw poll interval below one hour (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "gfw"))
	source := &Source{
		id: sourceConfig.ID, url: url, token: sourceConfig.ApiKey, interval: interval, now: time.Now,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "events.json"), TTL: interval},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "maritime.gfw" }

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	until := s.now().UTC().Format("2006-01-02")
	from := s.now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	rawURL := fmt.Sprintf("%s?datasets[0]=public-global-fishing-events:latest&start-date=%s&end-date=%s&limit=%d", s.url, from, until, maxEvents)
	return pollutil.Get(ctx, s.client, rawURL, map[string]string{"Authorization": "Bearer " + s.token})
}

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("gfw using disk cache", "error", err)
		return cached, nil
	}
	return nil, err
}

type gfwResponse struct {
	Entries []gfwEvent `json:"entries"`
	Events  []gfwEvent `json:"events"`
}

type gfwEvent struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Start    string `json:"start"`
	Position struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"position"`
	Fishing struct {
		TotalDurationHours float64 `json:"totalDurationHours"`
	} `json:"fishing"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	var response gfwResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode gfw: %w", err)
	}
	items := response.Entries
	if len(items) == 0 {
		items = response.Events
	}
	now := s.now().UTC()
	records := make([]plugins.RawRecord, 0, maxEvents)
	for _, item := range items {
		if item.Position.Lat == 0 && item.Position.Lon == 0 {
			continue
		}
		id := item.ID
		if id == "" {
			id = fmt.Sprintf("%.3f,%.3f", item.Position.Lat, item.Position.Lon)
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "fishing", "cellId": id, "presenceKind": item.Type,
			"hours":    item.Fishing.TotalDurationHours,
			"latitude": item.Position.Lat, "longitude": item.Position.Lon,
			"attribution": "Global Fishing Watch (CC BY-NC)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "gfw", SourceInstanceID: s.id, OriginalID: "gfw-" + id,
			Domain: "maritime", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxEvents {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("gfw sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("gfw snapshot", "source", s.id, "events", len(records))
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
