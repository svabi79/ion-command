// Package adsbfi polls the adsb.fi military snapshot only
// (https://opendata.adsb.fi/api/v2/mil). Personal non-commercial use.
// Attribute adsb.fi. Never the general civil snapshot. Poll no faster
// than one request per second.
package adsbfi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL     = "https://opendata.adsb.fi/api/v2/mil"
	pollDefault    = 30 * time.Second
	pollFloor      = time.Second
	requestSpacing = time.Second
	maxAircraft    = 200
)

var requestGate = struct {
	sync.Mutex
	last time.Time
}{}

func waitForRequestSlot(ctx context.Context) error {
	for {
		requestGate.Lock()
		wait := requestSpacing - time.Since(requestGate.last)
		if wait <= 0 {
			requestGate.last = time.Now()
			requestGate.Unlock()
			return nil
		}
		requestGate.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

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
	if sourceConfig.Type != "aviation.adsbfi" {
		return nil, fmt.Errorf("unsupported adsbfi source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("adsbfi poll interval below one second (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = strings.TrimSuffix(sourceConfig.Broker, "/")
	}
	if strings.Contains(strings.ToLower(url), "/snapshot") || strings.HasSuffix(strings.ToLower(url), "/v2/all") {
		return nil, fmt.Errorf("adsbfi refuses the civil snapshot; military endpoint only")
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "adsbfi"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "mil.json"), TTL: interval},
		client: &http.Client{Timeout: 40 * time.Second}, logger: logger,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.adsbfi" }

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	if err := waitForRequestSlot(ctx); err != nil {
		return nil, err
	}
	body, err := pollutil.Get(ctx, s.client, s.url, map[string]string{"Accept": "application/json"})
	if err != nil {
		var limited pollutil.RateLimitedError
		if errors.As(err, &limited) {
			return nil, limited
		}
		return nil, err
	}
	return body, nil
}

type milResponse struct {
	Aircraft []struct {
		Hex          string          `json:"hex"`
		Flight       string          `json:"flight"`
		Type         string          `json:"t"`
		Registration string          `json:"r"`
		Category     string          `json:"category"`
		Squawk       string          `json:"squawk"`
		BaroRateFpm  float64         `json:"baro_rate"`
		AltBaro      json.RawMessage `json:"alt_baro"`
		GsKt         float64         `json:"gs"`
		Track        float64         `json:"track"`
		Lat          *float64        `json:"lat"`
		Lon          *float64        `json:"lon"`
	} `json:"ac"`
}

func kindForReadsbCategory(category string) string {
	switch category {
	case "A7":
		return "helicopter"
	case "B1", "B4":
		return "glider"
	case "B2":
		return "balloon"
	case "B6":
		return "drone"
	default:
		return "aircraft"
	}
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("adsbfi using disk cache", "error", err)
			body = cached
		} else {
			return nil, err
		}
	} else {
		_ = s.cache.Store(body)
	}
	var response milResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode adsbfi: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxAircraft)
	for _, aircraft := range response.Aircraft {
		if aircraft.Hex == "" || aircraft.Lat == nil || aircraft.Lon == nil {
			continue
		}
		altFt := 0.0
		onGround := false
		if len(aircraft.AltBaro) > 0 {
			var numeric float64
			if err := json.Unmarshal(aircraft.AltBaro, &numeric); err == nil {
				altFt = numeric
			} else {
				onGround = true
			}
		}
		fields := map[string]any{
			"hex":          aircraft.Hex,
			"callsign":     strings.TrimSpace(aircraft.Flight),
			"acType":       aircraft.Type,
			"registration": aircraft.Registration,
			"kind":         kindForReadsbCategory(aircraft.Category),
			"squawk":       aircraft.Squawk,
			"baroRateFpm":  aircraft.BaroRateFpm,
			"lat":          *aircraft.Lat,
			"lon":          *aircraft.Lon,
			"altFt":        altFt,
			"gsKt":         aircraft.GsKt,
			"track":        aircraft.Track,
			"onGround":     onGround,
			"validSeconds": 90,
		}
		payload, err := json.Marshal(fields)
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "adsbfi", SourceInstanceID: s.id,
			OriginalID:  fmt.Sprintf("adsbfi-%s-%d", aircraft.Hex, now.Unix()),
			Domain:      "aviation",
			ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxAircraft {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("adsbfi sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("adsbfi snapshot", "source", s.id, "aircraft", len(records))
		}
		wait := s.interval
		var limited pollutil.RateLimitedError
		if errors.As(err, &limited) && limited.RetryAfter > wait {
			wait = limited.RetryAfter
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		if err := pollutil.Sleep(ctx, wait); err != nil {
			return nil
		}
	}
}
