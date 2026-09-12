// Package nhc polls NOAA NHC CurrentStorms.json for active tropical
// cyclone centres in the Atlantic and eastern Pacific. Positions are
// US Government work (public domain). Cones and wind radii stay out of
// scope until the renderer has Area geometry.
package nhc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://www.nhc.noaa.gov/CurrentStorms.json"
	pollDefault = 10 * time.Minute
	pollFloor   = 5 * time.Minute
	maxStorms   = 20
)

var classifications = map[string]string{
	"TD":  "Tropical Depression",
	"TS":  "Tropical Storm",
	"HU":  "Hurricane",
	"MH":  "Major Hurricane",
	"STS": "Subtropical Storm",
	"STD": "Subtropical Depression",
	"PT":  "Post-Tropical",
	"DB":  "Disturbance",
	"TY":  "Typhoon",
	"ST":  "Super Typhoon",
}

type Source struct {
	id       string
	url      string
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.nhc" {
		return nil, fmt.Errorf("unsupported nhc source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("nhc poll interval below five minutes (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	source := &Source{id: sourceConfig.ID, url: url, interval: interval, client: &http.Client{Timeout: 30 * time.Second}, logger: logger}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.nhc" }

type nhcResponse struct {
	ActiveStorms []nhcStorm `json:"activeStorms"`
}

type nhcStorm struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	Classification   string  `json:"classification"`
	Intensity        string  `json:"intensity"`
	Pressure         string  `json:"pressure"`
	LatitudeNumeric  float64 `json:"latitudeNumeric"`
	LongitudeNumeric float64 `json:"longitudeNumeric"`
	MovementDir      float64 `json:"movementDir"`
	MovementSpeed    float64 `json:"movementSpeed"`
	LastUpdate       string  `json:"lastUpdate"`
}

func parseNumber(text string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0
	}
	return value
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var response nhcResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode nhc response: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.ActiveStorms))
	for _, storm := range response.ActiveStorms {
		if storm.ID == "" {
			continue
		}
		if storm.LatitudeNumeric < -90 || storm.LatitudeNumeric > 90 || storm.LongitudeNumeric < -180 || storm.LongitudeNumeric > 180 {
			continue
		}
		class := strings.ToUpper(strings.TrimSpace(storm.Classification))
		label := classifications[class]
		if label == "" {
			label = class
		}
		name := strings.TrimSpace(storm.Name)
		if name == "" {
			name = storm.ID
		}
		payload, err := json.Marshal(map[string]any{
			"kind":           "storm",
			"stormId":        storm.ID,
			"name":           name,
			"classification": class,
			"classLabel":     label,
			"intensityKt":    parseNumber(storm.Intensity),
			"pressureHpa":    parseNumber(storm.Pressure),
			"latitude":       storm.LatitudeNumeric,
			"longitude":      storm.LongitudeNumeric,
			"motionDeg":      storm.MovementDir,
			"motionKt":       storm.MovementSpeed,
			"lastUpdate":     storm.LastUpdate,
			"provider":       "nhc",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "nhc", SourceInstanceID: s.id, OriginalID: storm.ID,
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxStorms {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("nhc sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("nhc snapshot", "source", s.id, "storms", len(records))
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
