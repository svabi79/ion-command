// Package reliefweb polls OCHA ReliefWeb v2 disasters and emits one
// record per current or alert disaster. Every request needs a
// pre-approved appname (since 1 November 2025): request one via the
// form linked from https://apidoc.reliefweb.int/parameters and put it
// in gitignored local.json as apiKey. The source refuses to start
// without one.
package reliefweb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultBase = "https://api.reliefweb.int/v2/disasters"
	pollDefault = 15 * time.Minute
	pollFloor   = 5 * time.Minute
	cacheName   = "disasters.json"
	maxEvents   = 80
)

type Source struct {
	id       string
	base     string
	appname  string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	now      func() time.Time
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "humanitarian.reliefweb" {
		return nil, fmt.Errorf("unsupported reliefweb source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	appname := strings.TrimSpace(sourceConfig.ApiKey)
	if appname == "" {
		return nil, fmt.Errorf("humanitarian.reliefweb requires apiKey (ReliefWeb pre-approved appname in local.json)")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("reliefweb poll interval below five minutes (got %s)", interval)
	}
	base := defaultBase
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "reliefweb"))
	source := &Source{
		id: sourceConfig.ID, base: base, appname: appname, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, cacheName)},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger, now: time.Now,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "humanitarian.reliefweb" }

func (s *Source) queryURL() string {
	values := url.Values{}
	values.Set("appname", s.appname)
	return s.base + "?" + values.Encode()
}

func (s *Source) requestBody() ([]byte, error) {
	payload := map[string]any{
		"limit": maxEvents,
		"sort":  []string{"date.created:desc"},
		"filter": map[string]any{
			"field":    "status",
			"value":    []string{"alert", "current"},
			"operator": "OR",
		},
		"fields": map[string]any{
			"include": []string{
				"id", "name", "status", "glide",
				"type.name", "type.code", "type.primary",
				"primary_country.name", "primary_country.iso3", "primary_country.location",
				"date.event",
			},
		},
	}
	return json.Marshal(payload)
}

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	body, err := s.requestBody()
	if err != nil {
		return nil, err
	}
	response, err := pollutil.Post(ctx, s.client, s.queryURL(), "application/json", body, nil)
	if err != nil {
		return nil, err
	}
	_ = s.cache.Store(response)
	return response, nil
}

func (s *Source) loadBody(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		return body, nil
	}
	if cached, _, ok := s.cache.Load(); ok {
		s.logger.Warn("reliefweb using disk cache", "source", s.id, "error", err)
		return cached, nil
	}
	return nil, err
}

type reliefwebResponse struct {
	Data []reliefwebItem `json:"data"`
}

type reliefwebItem struct {
	ID     json.RawMessage `json:"id"`
	Fields struct {
		ID             json.RawMessage `json:"id"`
		Name           string          `json:"name"`
		Status         string          `json:"status"`
		Glide          string          `json:"glide"`
		Type           []reliefwebType `json:"type"`
		PrimaryCountry struct {
			Name     string `json:"name"`
			ISO3     string `json:"iso3"`
			Location struct {
				Lat json.RawMessage `json:"lat"`
				Lon json.RawMessage `json:"lon"`
			} `json:"location"`
		} `json:"primary_country"`
		Date struct {
			Event string `json:"event"`
		} `json:"date"`
	} `json:"fields"`
}

type reliefwebType struct {
	Name    string `json:"name"`
	Code    string `json:"code"`
	Primary bool   `json:"primary"`
}

func asID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		return strings.TrimSpace(asString)
	}
	var asNumber float64
	if json.Unmarshal(raw, &asNumber) == nil {
		return strconv.FormatInt(int64(asNumber), 10)
	}
	return ""
}

func asFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var asNumber float64
	if json.Unmarshal(raw, &asNumber) == nil {
		return asNumber, true
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		value, err := strconv.ParseFloat(strings.TrimSpace(asString), 64)
		return value, err == nil
	}
	return 0, false
}

func primaryType(types []reliefwebType) (name, code string) {
	for _, entry := range types {
		if entry.Primary {
			return strings.TrimSpace(entry.Name), strings.TrimSpace(entry.Code)
		}
	}
	if len(types) > 0 {
		return strings.TrimSpace(types[0].Name), strings.TrimSpace(types[0].Code)
	}
	return "", ""
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.loadBody(ctx)
	if err != nil {
		return nil, err
	}
	var response reliefwebResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode reliefweb response: %w", err)
	}
	now := s.now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.Data))
	for _, item := range response.Data {
		id := asID(item.Fields.ID)
		if id == "" {
			id = asID(item.ID)
		}
		if id == "" {
			continue
		}
		lat, latOK := asFloat(item.Fields.PrimaryCountry.Location.Lat)
		lon, lonOK := asFloat(item.Fields.PrimaryCountry.Location.Lon)
		if !latOK || !lonOK {
			continue
		}
		category, categoryCode := primaryType(item.Fields.Type)
		payload, err := json.Marshal(map[string]any{
			"kind":         "disaster",
			"disasterId":   id,
			"title":        item.Fields.Name,
			"status":       item.Fields.Status,
			"glide":        item.Fields.Glide,
			"category":     category,
			"categoryCode": categoryCode,
			"country":      item.Fields.PrimaryCountry.Name,
			"countryCode":  strings.ToUpper(strings.TrimSpace(item.Fields.PrimaryCountry.ISO3)),
			"latitude":     lat,
			"longitude":    lon,
			"eventDate":    item.Fields.Date.Event,
			"attribution":  "ReliefWeb / OCHA",
			"provider":     "reliefweb",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "reliefweb", SourceInstanceID: s.id,
			OriginalID:  "reliefweb-" + id,
			Domain:      "humanitarian",
			ObservedUTC: now,
			Payload:     payload,
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
			s.logger.Warn("reliefweb sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("reliefweb snapshot", "source", s.id, "disasters", len(records))
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
