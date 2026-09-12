// Package pads polls The Space Devs Launch Library 2 locations list
// (https://ll.thespacedevs.com) for Earth spaceports with coordinates.
// A daily poll stays well inside the anonymous 15-calls/hour budget
// already used by space.launchlibrary.
package pads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://ll.thespacedevs.com/2.3.0/locations/?limit=100&format=json"
	pollDefault = 24 * time.Hour
	pollFloor   = 6 * time.Hour
	cacheName   = "locations.json"
	maxPads     = 80
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
	if sourceConfig.Type != "space.pads" {
		return nil, fmt.Errorf("unsupported pads source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("spaceport poll interval below six hours would crowd the Launch Library budget (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "launchlibrary"))
	source := &Source{
		id:       sourceConfig.ID,
		url:      url,
		token:    strings.TrimSpace(sourceConfig.ApiKey),
		interval: interval,
		cache:    pollutil.FileCache{Path: filepath.Join(cacheDir, cacheName)},
		client:   &http.Client{Timeout: 45 * time.Second},
		logger:   logger,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "space.pads" }

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	headers := map[string]string{}
	if s.token != "" {
		headers["Authorization"] = "Token " + s.token
	}
	body, err := pollutil.Get(ctx, s.client, s.url, headers)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

func (s *Source) loadBody(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		return body, nil
	}
	if cached, _, ok := s.cache.Load(); ok {
		s.logger.Warn("spaceport list using disk cache", "source", s.id, "error", err)
		return cached, nil
	}
	return nil, err
}

type llResponse struct {
	Results []llLocation `json:"results"`
}

type llLocation struct {
	ID        int             `json:"id"`
	Name      string          `json:"name"`
	Active    bool            `json:"active"`
	Latitude  json.RawMessage `json:"latitude"`
	Longitude json.RawMessage `json:"longitude"`
	Country   struct {
		Name       string `json:"name"`
		Alpha3Code string `json:"alpha_3_code"`
	} `json:"country"`
	CelestialBody struct {
		Name string `json:"name"`
	} `json:"celestial_body"`
	TotalLaunchCount int `json:"total_launch_count"`
}

func flexibleFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var asFloat float64
	if json.Unmarshal(raw, &asFloat) == nil {
		return asFloat, true
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		value, err := strconv.ParseFloat(strings.TrimSpace(asString), 64)
		if err == nil {
			return value, true
		}
	}
	return 0, false
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.loadBody(ctx)
	if err != nil {
		return nil, err
	}
	var response llResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode launch library locations: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.Results))
	for _, loc := range response.Results {
		if loc.ID == 0 || strings.TrimSpace(loc.Name) == "" {
			continue
		}
		if loc.CelestialBody.Name != "" && !strings.EqualFold(loc.CelestialBody.Name, "Earth") {
			continue
		}
		lon, lonOK := flexibleFloat(loc.Longitude)
		lat, latOK := flexibleFloat(loc.Latitude)
		if !lonOK || !latOK {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind":        "pad",
			"padId":       strconv.Itoa(loc.ID),
			"name":        loc.Name,
			"country":     loc.Country.Name,
			"countryCode": loc.Country.Alpha3Code,
			"latitude":    lat,
			"longitude":   lon,
			"active":      loc.Active,
			"launches":    loc.TotalLaunchCount,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "pads", SourceInstanceID: s.id,
			OriginalID:  fmt.Sprintf("ll2-loc-%d", loc.ID),
			Domain:      "space",
			ObservedUTC: now,
			Payload:     payload,
		})
		if len(records) >= maxPads {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("spaceport sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("spaceport snapshot", "source", s.id, "locations", len(records))
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		wait := s.interval
		var limited pollutil.RateLimitedError
		if errors.As(err, &limited) {
			wait = max(wait, limited.RetryAfter)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
	}
}
