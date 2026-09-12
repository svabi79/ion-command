// Package hapi polls HDX HAPI (OCHA) for the current-year UNHCR refugee
// totals and emits host and origin country points. Every request needs
// an app identifier (not a secret): mint one at
// https://hapi.humdata.org/api/v2/encode_app_identifier and put it in
// gitignored local.json. The source refuses to start without one.
package hapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultBase   = "https://hapi.humdata.org/api/v2/affected-people/refugees-persons-of-concern"
	pollDefault   = 24 * time.Hour
	pollFloor     = 6 * time.Hour
	cacheName     = "refugees.json"
	minPopulation = 100000
	maxHosts      = 25
	maxOrigins    = 25
	pageLimit     = 10000
)

type Source struct {
	id         string
	base       string
	identifier string
	interval   time.Duration
	cache      pollutil.FileCache
	client     *http.Client
	logger     *slog.Logger
	now        func() time.Time
	fetch      func(ctx context.Context, year int) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "humanitarian.hapi" {
		return nil, fmt.Errorf("unsupported hapi source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	identifier := strings.TrimSpace(sourceConfig.ApiKey)
	if identifier == "" {
		return nil, fmt.Errorf("humanitarian.hapi requires apiKey (HDX HAPI app identifier in local.json)")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("hapi poll interval below six hours (got %s)", interval)
	}
	base := defaultBase
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "hapi"))
	source := &Source{
		id: sourceConfig.ID, base: base, identifier: identifier, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, cacheName)},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger, now: time.Now,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "humanitarian.hapi" }

func (s *Source) queryURL(year int) string {
	values := url.Values{}
	values.Set("app_identifier", s.identifier)
	values.Set("start_date", strconv.Itoa(year))
	values.Set("population_group", "REF")
	values.Set("gender", "all")
	values.Set("age_range", "all")
	values.Set("limit", strconv.Itoa(pageLimit))
	return s.base + "?" + values.Encode()
}

func (s *Source) httpFetch(ctx context.Context, year int) ([]byte, error) {
	body, err := pollutil.Get(ctx, s.client, s.queryURL(year), map[string]string{
		"X-HDX-HAPI-APP-IDENTIFIER": s.identifier,
	})
	if err != nil {
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

func (s *Source) loadBody(ctx context.Context, year int) ([]byte, error) {
	body, err := s.fetch(ctx, year)
	if err == nil {
		return body, nil
	}
	if cached, _, ok := s.cache.Load(); ok {
		s.logger.Warn("hapi using disk cache", "source", s.id, "error", err)
		return cached, nil
	}
	return nil, err
}

type hapiResponse struct {
	Data []hapiRow `json:"data"`
}

type hapiRow struct {
	Population           int    `json:"population"`
	OriginLocationCode   string `json:"origin_location_code"`
	OriginLocationName   string `json:"origin_location_name"`
	AsylumLocationCode   string `json:"asylum_location_code"`
	AsylumLocationName   string `json:"asylum_location_name"`
	ReferencePeriodStart string `json:"reference_period_start"`
}

type countryTotal struct {
	Code       string
	Name       string
	Population int
}

func aggregate(rows []hapiRow, codeFn func(hapiRow) string, nameFn func(hapiRow) string) []countryTotal {
	index := map[string]*countryTotal{}
	for _, row := range rows {
		code := strings.ToUpper(strings.TrimSpace(codeFn(row)))
		if len(code) != 3 {
			continue
		}
		entry, ok := index[code]
		if !ok {
			entry = &countryTotal{Code: code, Name: strings.TrimSpace(nameFn(row))}
			index[code] = entry
		}
		entry.Population += row.Population
	}
	out := make([]countryTotal, 0, len(index))
	for _, entry := range index {
		if entry.Population >= minPopulation {
			out = append(out, *entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Population > out[j].Population })
	return out
}

func (s *Source) decodeYear(ctx context.Context, year int) ([]hapiRow, error) {
	body, err := s.loadBody(ctx, year)
	if err != nil {
		return nil, err
	}
	var response hapiResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode hapi response: %w", err)
	}
	return response.Data, nil
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	year := s.now().UTC().Year()
	rows, err := s.decodeYear(ctx, year)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		rows, err = s.decodeYear(ctx, year-1)
		if err != nil {
			return nil, err
		}
	}
	hosts := aggregate(rows, func(r hapiRow) string { return r.AsylumLocationCode }, func(r hapiRow) string { return r.AsylumLocationName })
	origins := aggregate(rows, func(r hapiRow) string { return r.OriginLocationCode }, func(r hapiRow) string { return r.OriginLocationName })
	now := s.now().UTC()
	records := make([]plugins.RawRecord, 0, maxHosts+maxOrigins)
	records = append(records, s.emit(now, "host", hosts, maxHosts)...)
	records = append(records, s.emit(now, "origin", origins, maxOrigins)...)
	return records, nil
}

func (s *Source) emit(now time.Time, role string, totals []countryTotal, limit int) []plugins.RawRecord {
	if len(totals) > limit {
		totals = totals[:limit]
	}
	records := make([]plugins.RawRecord, 0, len(totals))
	for _, total := range totals {
		lat, lon, name, ok := LookupCentroid(total.Code)
		if !ok {
			continue
		}
		if total.Name != "" {
			name = total.Name
		}
		payload, err := json.Marshal(map[string]any{
			"kind":        "displacement",
			"country":     total.Code,
			"name":        name,
			"role":        role,
			"population":  total.Population,
			"latitude":    lat,
			"longitude":   lon,
			"attribution": "UNHCR via HDX HAPI",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "hapi", SourceInstanceID: s.id,
			OriginalID:  "hapi-" + role + "-" + total.Code,
			Domain:      "humanitarian",
			ObservedUTC: now,
			Payload:     payload,
		})
	}
	return records
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("hapi sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("hapi snapshot", "source", s.id, "points", len(records))
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
