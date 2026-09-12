// Package openaq polls OpenAQ v3 locations near a configured point.
// A free API key is required (X-API-Key). The source stays disabled in
// the shipped configs until the operator puts a key in local.json.
package openaq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultBase = "https://api.openaq.org/v3/locations"
	pollDefault = 15 * time.Minute
	pollFloor   = 5 * time.Minute
	maxStations = 40
)

type Source struct {
	id        string
	base      string
	apiKey    string
	latitude  float64
	longitude float64
	radiusM   float64
	interval  time.Duration
	client    *http.Client
	logger    *slog.Logger
	fetch     func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.openaq" {
		return nil, fmt.Errorf("unsupported openaq source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(sourceConfig.ApiKey) == "" {
		return nil, fmt.Errorf("weather.openaq requires apiKey (OpenAQ Explorer key in local.json)")
	}
	if sourceConfig.Latitude == 0 && sourceConfig.Longitude == 0 {
		return nil, fmt.Errorf("weather.openaq requires latitude and longitude")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("openaq poll interval below five minutes (got %s)", interval)
	}
	radius := sourceConfig.RadiusNm * 1852
	if radius <= 0 {
		radius = 100000
	}
	base := defaultBase
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	source := &Source{
		id: sourceConfig.ID, base: base, apiKey: sourceConfig.ApiKey,
		latitude: sourceConfig.Latitude, longitude: sourceConfig.Longitude,
		radiusM: radius, interval: interval,
		client: &http.Client{Timeout: 30 * time.Second}, logger: logger,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.openaq" }

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	query := url.Values{}
	query.Set("coordinates", fmt.Sprintf("%s,%s", strconv.FormatFloat(s.latitude, 'f', 5, 64), strconv.FormatFloat(s.longitude, 'f', 5, 64)))
	query.Set("radius", strconv.FormatFloat(s.radiusM, 'f', 0, 64))
	query.Set("limit", strconv.Itoa(maxStations))
	return pollutil.Get(ctx, s.client, s.base+"?"+query.Encode(), map[string]string{"X-API-Key": s.apiKey})
}

type openaqResponse struct {
	Results []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Locality    string `json:"locality"`
		Coordinates struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"coordinates"`
		Country struct {
			Code string `json:"code"`
			Name string `json:"name"`
		} `json:"country"`
		DatetimeLast struct {
			UTC string `json:"utc"`
		} `json:"datetimeLast"`
		Sensors []struct {
			Name      string `json:"name"`
			Parameter struct {
				Name        string `json:"name"`
				Units       string `json:"units"`
				DisplayName string `json:"displayName"`
			} `json:"parameter"`
		} `json:"sensors"`
	} `json:"results"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var response openaqResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode openaq response: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.Results))
	for _, loc := range response.Results {
		if loc.ID == 0 || (loc.Coordinates.Latitude == 0 && loc.Coordinates.Longitude == 0) {
			continue
		}
		parameter := ""
		if len(loc.Sensors) > 0 {
			parameter = loc.Sensors[0].Parameter.DisplayName
			if parameter == "" {
				parameter = loc.Sensors[0].Parameter.Name
			}
		}
		payload, err := json.Marshal(map[string]any{
			"kind":        "airquality",
			"stationId":   loc.ID,
			"name":        loc.Name,
			"locality":    loc.Locality,
			"country":     loc.Country.Name,
			"latitude":    loc.Coordinates.Latitude,
			"longitude":   loc.Coordinates.Longitude,
			"parameter":   parameter,
			"lastUtc":     loc.DatetimeLast.UTC,
			"attribution": "Air quality data by OpenAQ",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "openaq", SourceInstanceID: s.id,
			OriginalID: fmt.Sprintf("openaq-%d", loc.ID),
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("openaq sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("openaq snapshot", "source", s.id, "stations", len(records))
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
