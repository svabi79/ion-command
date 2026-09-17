// Package ndbc polls NOAA NDBC latest observations (moored buoys and DART
// tsunami buoys) and emits Points. Public US Government work.
package ndbc

import (
	"context"
	"encoding/json"
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
	defaultURL  = "https://www.ndbc.noaa.gov/data/latest_obs/latest_obs.txt"
	pollDefault = 10 * time.Minute
	pollFloor   = 5 * time.Minute
	maxPoints   = 180
	maxDART     = 40
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
	if sourceConfig.Type != "weather.ndbc" {
		return nil, fmt.Errorf("unsupported ndbc source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("ndbc poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "ndbc"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "latest_obs.txt"), TTL: interval},
		client: &http.Client{Timeout: 30 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.ndbc" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		_ = s.cache.Store(body)
		return body, nil
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("ndbc using disk cache", "error", err)
		return cached, nil
	}
	return nil, err
}

func parseObs(text string) float64 {
	text = strings.TrimSpace(text)
	if text == "" || strings.EqualFold(text, "MM") {
		return 0
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0
	}
	return value
}

func isDART(id string) bool {
	if len(id) != 5 {
		return false
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return false
		}
	}
	switch id[:2] {
	case "21", "23", "32", "43", "44", "46", "51", "52", "55":
		return true
	}
	return strings.HasPrefix(id, "4142")
}

type station struct {
	id, kind         string
	lat, lon         float64
	wind, wave, pres float64
	water            float64
	dart             bool
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(body), "\n")
	var dart, buoys []station
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 16 {
			continue
		}
		lat, errLat := strconv.ParseFloat(fields[1], 64)
		lon, errLon := strconv.ParseFloat(fields[2], 64)
		if errLat != nil || errLon != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			continue
		}
		id := fields[0]
		item := station{
			id: id, lat: lat, lon: lon,
			wind: parseObs(fields[9]), wave: parseObs(fields[11]), pres: parseObs(fields[15]),
		}
		if len(fields) > 18 {
			item.water = parseObs(fields[18])
		}
		if isDART(id) {
			item.kind = "DART"
			item.dart = true
			dart = append(dart, item)
			continue
		}
		item.kind = "buoy"
		buoys = append(buoys, item)
	}
	if len(dart) > maxDART {
		dart = dart[:maxDART]
	}
	picked := append([]station{}, dart...)
	remain := maxPoints - len(picked)
	if remain < 0 {
		remain = 0
	}
	if len(buoys) > remain {
		buoys = buoys[:remain]
	}
	picked = append(picked, buoys...)
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(picked))
	for _, item := range picked {
		payload, err := json.Marshal(map[string]any{
			"kind": "buoy", "stationId": item.id, "name": item.id, "buoyKind": item.kind,
			"windMs": item.wind, "waveM": item.wave, "pressureHpa": item.pres,
			"waterTempC": item.water, "latitude": item.lat, "longitude": item.lon,
			"attribution": "NOAA National Data Buoy Center",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "ndbc", SourceInstanceID: s.id, OriginalID: "ndbc-" + item.id,
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("ndbc sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("ndbc snapshot", "source", s.id, "points", len(records))
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
