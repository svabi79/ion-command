// Package ioda polls IODA internet-outage alerts for country entities and
// places them at ISO centroids. Georgia Tech copyright; hobby display only,
// do not republish the cache. ASN alerts are skipped (no geography).
package ioda

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
	defaultURL  = "https://api.ioda.inetintel.cc.gatech.edu/v2/outages/alerts"
	pollDefault = 15 * time.Minute
	pollFloor   = 5 * time.Minute
	maxOutages  = 80
	lookBack    = 6 * time.Hour
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
	if sourceConfig.Type != "geography.ioda" {
		return nil, fmt.Errorf("unsupported ioda source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("ioda poll interval below five minutes (got %s)", interval)
	}
	base := defaultURL
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "ioda"))
	source := &Source{
		id: sourceConfig.ID, url: base, interval: interval, now: time.Now,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "alerts.json"), TTL: interval},
		client: &http.Client{Timeout: 30 * time.Second}, logger: logger,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.ioda" }

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	until := s.now().UTC().Unix()
	from := until - int64(lookBack.Seconds())
	rawURL := fmt.Sprintf("%s?from=%d&until=%d&entityType=country&limit=100", s.url, from, until)
	return pollutil.Get(ctx, s.client, rawURL, nil)
}

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("ioda using disk cache", "error", err)
		return cached, nil
	}
	return nil, err
}

type iodaResponse struct {
	Data []iodaAlert `json:"data"`
}

type iodaAlert struct {
	Datasource string `json:"datasource"`
	Level      string `json:"level"`
	Time       int64  `json:"time"`
	Entity     struct {
		Code  string `json:"code"`
		Name  string `json:"name"`
		Type  string `json:"type"`
		Attrs struct {
			CountryCode string `json:"country_code"`
			CountryName string `json:"country_name"`
		} `json:"attrs"`
	} `json:"entity"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	var response iodaResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode ioda: %w", err)
	}
	now := s.now().UTC()
	seen := map[string]struct{}{}
	records := make([]plugins.RawRecord, 0, maxOutages)
	for _, alert := range response.Data {
		level := strings.ToLower(alert.Level)
		if level == "" || level == "normal" {
			continue
		}
		code := alert.Entity.Code
		if alert.Entity.Type != "country" {
			code = alert.Entity.Attrs.CountryCode
		}
		lat, lon, name, ok := iso3166.Lookup(code)
		if !ok {
			continue
		}
		if alert.Entity.Name != "" {
			name = alert.Entity.Name
		}
		id := strings.ToUpper(code) + "-" + level
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		payload, err := json.Marshal(map[string]any{
			"kind": "outage", "outageId": id, "name": name, "level": alert.Level,
			"country": strings.ToUpper(code), "latitude": lat, "longitude": lon,
			"attribution": "IODA / Georgia Tech Research Corporation",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "ioda", SourceInstanceID: s.id, OriginalID: "ioda-" + id,
			Domain: "geography", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxOutages {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("ioda sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("ioda snapshot", "source", s.id, "outages", len(records))
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
