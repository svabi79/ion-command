// Package swpc polls NOAA SWPC JSON products and emits measured space-weather
// samples: planetary Kp, 10.7 cm solar flux, solar wind speed/density, IMF Bz.
package swpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	kpURL       = "https://services.swpc.noaa.gov/products/noaa-planetary-k-index.json"
	fluxURL     = "https://services.swpc.noaa.gov/products/summary/10cm-flux.json"
	windURL     = "https://services.swpc.noaa.gov/products/summary/solar-wind-speed.json"
	magURL      = "https://services.swpc.noaa.gov/products/summary/solar-wind-mag-field.json"
	wwvURL      = "https://services.swpc.noaa.gov/text/wwv.txt"
	xrayURL     = "https://services.swpc.noaa.gov/json/goes/primary/xrays-6-hour.json"
	pollDefault = 5 * time.Minute
)

// wwv.txt carries the only machine-readable estimated planetary A-index SWPC
// publishes ("Solar flux 110 and estimated planetary A-index 4.").
var aIndexPattern = regexp.MustCompile(`estimated planetary A-index (\d+)`)

type Source struct {
	id       string
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger
	sequence uint64
	// fetch is swappable for tests.
	fetch func(ctx context.Context, url string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "spaceweather.swpc" {
		return nil, fmt.Errorf("unsupported SWPC source type %q", sourceConfig.Type)
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < time.Minute {
		return nil, fmt.Errorf("SWPC poll interval below one minute would hammer NOAA (got %s)", interval)
	}
	if logger == nil {
		logger = slog.Default()
	}
	source := &Source{id: sourceConfig.ID, interval: interval, client: &http.Client{Timeout: 20 * time.Second}, logger: logger}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "spaceweather.swpc" }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		record, err := s.sample(ctx)
		if err != nil {
			s.logger.Warn("SWPC sample failed", "error", err)
		} else {
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

func (s *Source) httpFetch(ctx context.Context, url string) ([]byte, error) {
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
	return io.ReadAll(io.LimitReader(response.Body, 4<<20))
}

func (s *Source) sample(ctx context.Context) (plugins.RawRecord, error) {
	kp, err := s.latestKp(ctx)
	if err != nil {
		return plugins.RawRecord{}, err
	}
	flux := s.summaryValue(ctx, fluxURL, "flux")
	windSpeed := s.summaryValue(ctx, windURL, "proton_speed")
	bz := s.summaryValue(ctx, magURL, "bz_gsm")
	aIndex := s.estimatedAIndex(ctx)
	xrayFlux, xrayClass := s.xray(ctx)

	s.sequence++
	payload, err := json.Marshal(map[string]any{
		"sampleId":          fmt.Sprintf("swpc-%d", s.sequence),
		"kp":                kp,
		"aIndex":            aIndex,
		"solarFlux":         flux,
		"solarWindSpeedKms": windSpeed,
		"solarWindDensity":  nil,
		"imfBzNt":           bz,
		"xrayFluxWm2":       xrayFlux,
		"xrayClass":         xrayClass,
	})
	if err != nil {
		return plugins.RawRecord{}, err
	}
	return plugins.RawRecord{
		SourcePluginID:   "swpc",
		SourceInstanceID: s.id,
		OriginalID:       fmt.Sprintf("swpc-%s-%d", s.id, s.sequence),
		Domain:           "spaceweather",
		ObservedUTC:      time.Now().UTC(),
		Payload:          payload,
	}, nil
}

// latestKp parses the planetary-Kp product: an array of objects with a
// numeric (occasionally string) "Kp" per entry; the last entry is current.
func (s *Source) latestKp(ctx context.Context) (float64, error) {
	body, err := s.fetch(ctx, kpURL)
	if err != nil {
		return 0, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return 0, fmt.Errorf("decode Kp product: %w", err)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("Kp product has no entries")
	}
	switch value := rows[len(rows)-1]["Kp"].(type) {
	case float64:
		return value, nil
	case string:
		kp, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return 0, fmt.Errorf("parse Kp %q: %w", value, err)
		}
		return kp, nil
	default:
		return 0, fmt.Errorf("unexpected Kp type %T", rows[len(rows)-1]["Kp"])
	}
}

// estimatedAIndex parses the daily estimated planetary A-index out of the
// wwv.txt geophysical alert message. Failures degrade to null instead of
// failing the whole sample.
func (s *Source) estimatedAIndex(ctx context.Context) any {
	body, err := s.fetch(ctx, wwvURL)
	if err != nil {
		s.logger.Warn("SWPC wwv fetch failed", "url", wwvURL, "error", err)
		return nil
	}
	match := aIndexPattern.FindSubmatch(body)
	if match == nil {
		s.logger.Warn("SWPC wwv message has no estimated planetary A-index", "url", wwvURL)
		return nil
	}
	aIndex, err := strconv.ParseFloat(string(match[1]), 64)
	if err != nil {
		return nil
	}
	return aIndex
}

// xray reads the latest GOES long-wave (0.1-0.8 nm) X-ray flux and derives
// the flare class (A/B/C/M/X with a decimal multiplier, e.g. "B7.8").
// Failures degrade to null instead of failing the whole sample.
func (s *Source) xray(ctx context.Context) (any, any) {
	body, err := s.fetch(ctx, xrayURL)
	if err != nil {
		s.logger.Warn("SWPC xray fetch failed", "error", err)
		return nil, nil
	}
	var entries []struct {
		Flux   float64 `json:"flux"`
		Energy string  `json:"energy"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		s.logger.Warn("SWPC xray decode failed", "error", err)
		return nil, nil
	}
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].Energy != "0.1-0.8nm" || entries[index].Flux <= 0 {
			continue
		}
		flux := entries[index].Flux
		classes := []struct {
			letter string
			base   float64
		}{{"X", 1e-4}, {"M", 1e-5}, {"C", 1e-6}, {"B", 1e-7}, {"A", 1e-8}}
		for _, class := range classes {
			if flux >= class.base {
				return flux, fmt.Sprintf("%s%.1f", class.letter, flux/class.base)
			}
		}
		return flux, "A0.1"
	}
	return nil, nil
}

// summaryValue reads one numeric field from a SWPC summary product (an array
// of objects; the last entry is current). Failures degrade to null instead of
// failing the whole sample.
func (s *Source) summaryValue(ctx context.Context, url, field string) any {
	body, err := s.fetch(ctx, url)
	if err != nil {
		s.logger.Warn("SWPC summary fetch failed", "url", url, "error", err)
		return nil
	}
	var entries []map[string]any
	if err := json.Unmarshal(body, &entries); err != nil || len(entries) == 0 {
		s.logger.Warn("SWPC summary decode failed", "url", url, "error", err)
		return nil
	}
	switch value := entries[len(entries)-1][field].(type) {
	case string:
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
		return nil
	case float64:
		return value
	default:
		return nil
	}
}
