// Package adsb polls an ADS-B aggregator point query (default api.adsb.lol,
// a free community aggregator) for aircraft around a configured position.
// The same payload shape works for any readsb-style /v2/point endpoint, so a
// local receiver network can replace the aggregator via the broker override.
package adsb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	defaultBase   = "https://api.adsb.lol"
	pollDefault   = 20 * time.Second
	pollFloor     = 10 * time.Second
	defaultLat    = 47.3
	defaultLon    = 8.5
	defaultRadius = 200
	// Minimum spacing between requests across ALL instances: five area
	// queries from one IP tripped the aggregator's rate limit (HTTP 429,
	// 35 s responses) during the EU morning peak, which starved three of
	// the five circles entirely.
	requestSpacing = 4 * time.Second
	backoffInitial = 30 * time.Second
	backoffMax     = 5 * time.Minute
)

// requestGate serializes and spaces requests from every adsb source instance
// so simultaneous tickers cannot burst the shared aggregator.
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

// errRateLimited carries the server-requested pause from a 429 response.
type errRateLimited struct{ retryAfter time.Duration }

func (e errRateLimited) Error() string {
	return fmt.Sprintf("rate limited by aggregator (retry after %s)", e.retryAfter)
}

type pointResponse struct {
	Aircraft []struct {
		Hex     string          `json:"hex"`
		Flight  string          `json:"flight"`
		Type    string          `json:"t"`
		AltBaro json.RawMessage `json:"alt_baro"` // number of feet, or the string "ground"
		GsKt    float64         `json:"gs"`
		Track   float64         `json:"track"`
		Lat     *float64        `json:"lat"`
		Lon     *float64        `json:"lon"`
	} `json:"ac"`
}

type Source struct {
	id       string
	base     string
	lat, lon float64
	radius   float64
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger
	// fetch is swappable for tests.
	fetch func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "aviation.adsb" {
		return nil, fmt.Errorf("unsupported adsb source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("adsb poll interval below ten seconds would hammer the aggregator (got %s)", interval)
	}
	source := &Source{
		id:       sourceConfig.ID,
		base:     defaultBase,
		lat:      defaultLat,
		lon:      defaultLon,
		radius:   defaultRadius,
		interval: interval,
		client:   &http.Client{Timeout: 90 * time.Second},
		logger:   logger,
	}
	if sourceConfig.Broker != "" {
		source.base = strings.TrimSuffix(sourceConfig.Broker, "/")
	}
	if sourceConfig.Latitude != 0 || sourceConfig.Longitude != 0 {
		source.lat, source.lon = sourceConfig.Latitude, sourceConfig.Longitude
	}
	if sourceConfig.RadiusNm > 0 {
		source.radius = sourceConfig.RadiusNm
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.adsb" }

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	if err := waitForRequestSlot(ctx); err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/v2/point/%.4f/%.4f/%.0f", s.base, s.lat, s.lon, s.radius)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	// adsb.lol answers throttled clients with 420 "Enhance Your Calm";
	// treat it exactly like the standard 429.
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == 420 {
		retryAfter := backoffInitial
		if header := response.Header.Get("Retry-After"); header != "" {
			if seconds, parseErr := strconv.Atoi(strings.TrimSpace(header)); parseErr == nil && seconds > 0 {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
		return nil, errRateLimited{retryAfter: retryAfter}
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 16<<20))
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var response pointResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode adsb response: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.Aircraft))
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
		payload, err := json.Marshal(map[string]any{
			"hex":      aircraft.Hex,
			"callsign": strings.TrimSpace(aircraft.Flight),
			"acType":   aircraft.Type,
			"lat":      *aircraft.Lat,
			"lon":      *aircraft.Lon,
			"altFt":    altFt,
			"gsKt":     aircraft.GsKt,
			"track":    aircraft.Track,
			"onGround": onGround,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID:   "adsb",
			SourceInstanceID: s.id,
			OriginalID:       fmt.Sprintf("adsb-%s-%s-%d", s.id, aircraft.Hex, now.Unix()),
			Domain:           "aviation",
			ObservedUTC:      now,
			Payload:          payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	// Plain sleep instead of a ticker: when the aggregator answers slower
	// than the interval, an already-fired ticker would trigger the next
	// request back-to-back and feed the very rate limit that slowed us down.
	backoff := time.Duration(0)
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("adsb sample failed", "source", s.id, "error", err)
		}
		var limited errRateLimited
		if errors.As(err, &limited) {
			if backoff == 0 {
				backoff = limited.retryAfter
			} else {
				backoff = min(backoff*2, backoffMax)
			}
			backoff = max(backoff, limited.retryAfter)
		} else if err == nil {
			backoff = 0
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		pause := s.interval + backoff
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pause):
		}
	}
}
