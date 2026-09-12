// Package launchlibrary polls The Space Devs Launch Library 2 upcoming
// launches (https://ll.thespacedevs.com). Anonymous access is limited to
// 15 calls/hour, so the source polls at most every 15 minutes and serves
// the last good response from disk when the provider throttles.
package launchlibrary

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
	defaultURL  = "https://ll.thespacedevs.com/2.3.0/launches/upcoming/?limit=30&mode=normal"
	pollDefault = 15 * time.Minute
	pollFloor   = 15 * time.Minute
	cacheName   = "upcoming.json"
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
	if sourceConfig.Type != "space.launchlibrary" {
		return nil, fmt.Errorf("unsupported launch library source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("launch library poll interval below 15 minutes would exceed the anonymous 15/hour budget (got %s)", interval)
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
func (s *Source) Type() string { return "space.launchlibrary" }

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
		s.logger.Warn("launch library using disk cache", "source", s.id, "error", err)
		return cached, nil
	}
	return nil, err
}

type llResponse struct {
	Results []llLaunch `json:"results"`
}

type llLaunch struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Net    string `json:"net"`
	Status struct {
		Name   string `json:"name"`
		Abbrev string `json:"abbrev"`
	} `json:"status"`
	Pad struct {
		Name      string      `json:"name"`
		Latitude  json.RawMessage `json:"latitude"`
		Longitude json.RawMessage `json:"longitude"`
		Location  struct {
			Name string `json:"name"`
		} `json:"location"`
	} `json:"pad"`
	Rocket struct {
		Configuration struct {
			FullName string `json:"full_name"`
			Name     string `json:"name"`
		} `json:"configuration"`
	} `json:"rocket"`
	Mission struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"mission"`
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
		return nil, fmt.Errorf("decode launch library response: %w", err)
	}
	now := time.Now().UTC()
	horizon := now.Add(30 * 24 * time.Hour)
	records := make([]plugins.RawRecord, 0, len(response.Results))
	for _, launch := range response.Results {
		if launch.ID == "" {
			continue
		}
		lon, lonOK := flexibleFloat(launch.Pad.Longitude)
		lat, latOK := flexibleFloat(launch.Pad.Latitude)
		if !lonOK || !latOK {
			continue
		}
		if launch.Net != "" {
			if netTime, err := time.Parse(time.RFC3339, launch.Net); err == nil && netTime.After(horizon) {
				continue
			}
		}
		rocket := launch.Rocket.Configuration.FullName
		if rocket == "" {
			rocket = launch.Rocket.Configuration.Name
		}
		payload, err := json.Marshal(map[string]any{
			"launchId":   launch.ID,
			"name":       launch.Name,
			"status":     launch.Status.Name,
			"statusAbbrev": launch.Status.Abbrev,
			"net":        launch.Net,
			"padName":    launch.Pad.Name,
			"padLocation": launch.Pad.Location.Name,
			"latitude":   lat,
			"longitude":  lon,
			"rocket":     rocket,
			"mission":    launch.Mission.Name,
			"missionType": launch.Mission.Type,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID:   "launchlibrary",
			SourceInstanceID: s.id,
			OriginalID:       "ll2-" + launch.ID,
			Domain:           "space",
			ObservedUTC:      now,
			Payload:          payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("launch library sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("launch library snapshot", "source", s.id, "launches", len(records))
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
