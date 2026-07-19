// Package adsb polls an ADS-B aggregator point query (default api.adsb.lol,
// a free community aggregator) for aircraft around a configured position.
// The same payload shape works for any readsb-style /v2/point endpoint, so a
// local receiver network can replace the aggregator via the broker override.
package adsb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
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
)

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
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("adsb sample failed", "error", err)
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
		case <-ticker.C:
		}
	}
}
