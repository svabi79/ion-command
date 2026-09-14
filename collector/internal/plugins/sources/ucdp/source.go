// Package ucdp polls UCDP GED events. Token required
// (x-ucdp-access-token). Fail-closed without a key. Recent page only.
package ucdp

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
	defaultURL  = "https://ucdpapi.pcr.uu.se/api/gedevents/26.1?pagesize=100&page=0"
	pollDefault = 6 * time.Hour
	pollFloor   = time.Hour
	maxEvents   = 80
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
	if sourceConfig.Type != "conflict.ucdp" {
		return nil, fmt.Errorf("unsupported ucdp source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(sourceConfig.ApiKey) == "" {
		return nil, fmt.Errorf("conflict.ucdp requires apiKey (UCDP token in local.json)")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("ucdp poll interval below one hour (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "ucdp"))
	source := &Source{
		id: sourceConfig.ID, url: url, token: sourceConfig.ApiKey, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "ged.json"), TTL: interval},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.url, map[string]string{"x-ucdp-access-token": source.token})
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "conflict.ucdp" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("ucdp using disk cache", "error", err)
		return cached, nil
	}
	return nil, err
}

type ucdpResponse struct {
	Result []ucdpEvent `json:"Result"`
}

type ucdpEvent struct {
	ID               int     `json:"id"`
	RelID            string  `json:"relid"`
	ConflictName     string  `json:"conflict_name"`
	TypeOfViolence   int     `json:"type_of_violence"`
	Country          string  `json:"country"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	Best             int     `json:"best"`
	DateStart        string  `json:"date_start"`
	WhereCoordinates string  `json:"where_coordinates"`
}

func violenceKind(code int) string {
	switch code {
	case 1:
		return "state-based"
	case 2:
		return "non-state"
	case 3:
		return "one-sided"
	default:
		return "organized violence"
	}
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	var response ucdpResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode ucdp: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxEvents)
	for _, item := range response.Result {
		if item.Latitude == 0 && item.Longitude == 0 {
			continue
		}
		id := item.RelID
		if id == "" {
			id = fmt.Sprintf("%d", item.ID)
		}
		if id == "" {
			continue
		}
		title := item.WhereCoordinates
		if item.ConflictName != "" {
			title = item.ConflictName
		}
		payload, err := json.Marshal(map[string]any{
			"eventId": id, "title": title, "eventKind": violenceKind(item.TypeOfViolence),
			"country": item.Country, "deaths": item.Best,
			"latitude": item.Latitude, "longitude": item.Longitude,
			"attribution": "UCDP GED",
		})
		if err != nil {
			continue
		}
		observed := now
		if item.DateStart != "" {
			if parsed, err := time.Parse("2006-01-02T15:04:05", strings.TrimSuffix(item.DateStart, "Z")); err == nil {
				observed = parsed.UTC()
			}
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "ucdp", SourceInstanceID: s.id, OriginalID: "ucdp-" + id,
			Domain: "conflict", ObservedUTC: observed, Payload: payload,
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
			s.logger.Warn("ucdp sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("ucdp snapshot", "source", s.id, "events", len(records))
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
