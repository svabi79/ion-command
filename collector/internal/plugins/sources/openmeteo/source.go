// Package openmeteo polls current weather from Open-Meteo
// (https://open-meteo.com). Positions are rounded to 0.1° cells and
// answers are cached for five minutes. Attribution is required:
// "Weather data by Open-Meteo.com".
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultBase   = "https://api.open-meteo.com/v1/forecast"
	pollDefault   = 5 * time.Minute
	pollFloor     = 5 * time.Minute
	cellDegrees   = 0.1
	cacheTTL      = 5 * time.Minute
	attribution   = "Weather data by Open-Meteo.com"
)

type cacheEntry struct {
	body      []byte
	expiresAt time.Time
}

type Source struct {
	id        string
	base      string
	latitude  float64
	longitude float64
	interval  time.Duration
	client    *http.Client
	logger    *slog.Logger
	fetch     func(ctx context.Context, lat, lon float64) ([]byte, error)
	mu        sync.Mutex
	cells     map[string]cacheEntry
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.openmeteo" {
		return nil, fmt.Errorf("unsupported open-meteo source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if sourceConfig.Latitude == 0 && sourceConfig.Longitude == 0 {
		return nil, fmt.Errorf("weather.openmeteo requires latitude and longitude")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("open-meteo poll interval below five minutes (got %s)", interval)
	}
	base := defaultBase
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	source := &Source{
		id:        sourceConfig.ID,
		base:      base,
		latitude:  sourceConfig.Latitude,
		longitude: sourceConfig.Longitude,
		interval:  interval,
		client:    &http.Client{Timeout: 20 * time.Second},
		logger:    logger,
		cells:     make(map[string]cacheEntry),
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.openmeteo" }

func RoundCell(value float64) float64 {
	return math.Round(value/cellDegrees) * cellDegrees
}

func cellKey(lat, lon float64) string {
	return fmt.Sprintf("%.1f,%.1f", RoundCell(lat), RoundCell(lon))
}

func (s *Source) httpFetch(ctx context.Context, lat, lon float64) ([]byte, error) {
	query := url.Values{}
	query.Set("latitude", strconv.FormatFloat(lat, 'f', 1, 64))
	query.Set("longitude", strconv.FormatFloat(lon, 'f', 1, 64))
	query.Set("current", "temperature_2m,weather_code,wind_speed_10m,wind_direction_10m,relative_humidity_2m")
	return pollutil.Get(ctx, s.client, s.base+"?"+query.Encode(), nil)
}

func (s *Source) cachedFetch(ctx context.Context, lat, lon float64) ([]byte, error) {
	lat, lon = RoundCell(lat), RoundCell(lon)
	key := cellKey(lat, lon)
	now := time.Now()
	s.mu.Lock()
	if entry, ok := s.cells[key]; ok && now.Before(entry.expiresAt) {
		body := entry.body
		s.mu.Unlock()
		return body, nil
	}
	s.mu.Unlock()
	body, err := s.fetch(ctx, lat, lon)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cells[key] = cacheEntry{body: body, expiresAt: now.Add(cacheTTL)}
	s.mu.Unlock()
	return body, nil
}

type meteoResponse struct {
	Current struct {
		Time                string  `json:"time"`
		Temperature2M       float64 `json:"temperature_2m"`
		WeatherCode         int     `json:"weather_code"`
		WindSpeed10M        float64 `json:"wind_speed_10m"`
		WindDirection10M    float64 `json:"wind_direction_10m"`
		RelativeHumidity2M  float64 `json:"relative_humidity_2m"`
	} `json:"current"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	lat, lon := RoundCell(s.latitude), RoundCell(s.longitude)
	body, err := s.cachedFetch(ctx, lat, lon)
	if err != nil {
		return nil, err
	}
	var response meteoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode open-meteo response: %w", err)
	}
	now := time.Now().UTC()
	payload, err := json.Marshal(map[string]any{
		"kind":           "observation",
		"cellLat":        lat,
		"cellLon":        lon,
		"temperatureC":   response.Current.Temperature2M,
		"weatherCode":    response.Current.WeatherCode,
		"windKmh":        response.Current.WindSpeed10M,
		"windDirection":  response.Current.WindDirection10M,
		"humidityPct":    response.Current.RelativeHumidity2M,
		"observedAt":     response.Current.Time,
		"attribution":    attribution,
	})
	if err != nil {
		return nil, err
	}
	return []plugins.RawRecord{{
		SourcePluginID:   "openmeteo",
		SourceInstanceID: s.id,
		OriginalID:       fmt.Sprintf("openmeteo-%s-%s", s.id, cellKey(lat, lon)),
		Domain:           "weather",
		ObservedUTC:      now,
		Payload:          payload,
	}}, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("open-meteo sample failed", "source", s.id, "error", err)
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.interval):
		}
	}
}
