// Package cloudflare polls Cloudflare Radar outage annotations and places
// them at ISO country centroids. Bearer token required. Fail-closed.
package cloudflare

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
	"github.com/ion-command/ion-command/collector/internal/iso3166"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://api.cloudflare.com/client/v4/radar/annotations/outages?limit=40"
	pollDefault = 15 * time.Minute
	pollFloor   = 5 * time.Minute
	maxOutages  = 40
)

type Source struct {
	id       string
	url      string
	token    string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "geography.cloudflare" {
		return nil, fmt.Errorf("unsupported cloudflare source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(sourceConfig.ApiKey) == "" {
		return nil, fmt.Errorf("geography.cloudflare requires apiKey (Radar bearer token in local.json)")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("cloudflare poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "cloudflare"))
	source := &Source{
		id: sourceConfig.ID, url: url, token: sourceConfig.ApiKey, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "outages.json"), TTL: interval},
		client: &http.Client{Timeout: 30 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.url, map[string]string{"Authorization": "Bearer " + source.token})
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.cloudflare" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("cloudflare using disk cache", "error", err)
		return cached, nil
	}
	return nil, err
}

type cfResponse struct {
	Success bool `json:"success"`
	Result  struct {
		Annotations []cfAnnotation `json:"annotations"`
		Outages     []cfAnnotation `json:"outages"`
	} `json:"result"`
}

type cfAnnotation struct {
	ID          string `json:"id"`
	EventType   string `json:"eventType"`
	Description string `json:"description"`
	StartDate   string `json:"startDate"`
	Locations   []struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"locations"`
	Asns []struct {
		Name string `json:"name"`
	} `json:"asns"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	var response cfResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode cloudflare radar: %w", err)
	}
	items := response.Result.Annotations
	if len(items) == 0 {
		items = response.Result.Outages
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxOutages)
	seen := map[string]struct{}{}
	for _, item := range items {
		for _, loc := range item.Locations {
			lat, lon, name, ok := iso3166.Lookup(loc.Code)
			if !ok {
				continue
			}
			if loc.Name != "" {
				name = loc.Name
			}
			id := item.ID + "-" + strings.ToUpper(loc.Code)
			if item.ID == "" {
				id = strings.ToUpper(loc.Code) + "-" + item.StartDate
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			title := item.Description
			if title == "" {
				title = name
			}
			payload, err := json.Marshal(map[string]any{
				"kind": "outage", "outageId": id, "name": title, "level": "major",
				"country": strings.ToUpper(loc.Code), "latitude": lat, "longitude": lon,
				"attribution": "Cloudflare Radar",
			})
			if err != nil {
				continue
			}
			records = append(records, plugins.RawRecord{
				SourcePluginID: "cloudflare", SourceInstanceID: s.id, OriginalID: "cf-" + id,
				Domain: "geography", ObservedUTC: now, Payload: payload,
			})
			if len(records) >= maxOutages {
				return records, nil
			}
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("cloudflare sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("cloudflare snapshot", "source", s.id, "outages", len(records))
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
