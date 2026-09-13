// Package nhc polls NOAA NHC CurrentStorms.json for active tropical
// cyclone centres in the Atlantic and eastern Pacific, then fetches the
// 5-day forecast cone KMZ linked from each storm's trackCone product.
// Positions and cones are US Government work (public domain).
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
	fetch    func(ctx context.Context, rawURL string) ([]byte, error)
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
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, rawURL, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.nhc" }

type nhcResponse struct {
	ActiveStorms []nhcStorm `json:"activeStorms"`
}

type nhcProduct struct {
	AdvNum   string `json:"advNum"`
	Issuance string `json:"issuance"`
	KmzFile  string `json:"kmzFile"`
	ZipFile  string `json:"zipFile"`
}

type nhcStorm struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Classification   string      `json:"classification"`
	Intensity        string      `json:"intensity"`
	Pressure         string      `json:"pressure"`
	LatitudeNumeric  float64     `json:"latitudeNumeric"`
	LongitudeNumeric float64     `json:"longitudeNumeric"`
	MovementDir      float64     `json:"movementDir"`
	MovementSpeed    float64     `json:"movementSpeed"`
	LastUpdate       string      `json:"lastUpdate"`
	TrackCone        *nhcProduct `json:"trackCone"`
}

func parseNumber(text string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0
	}
	return value
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx, s.url)
	if err != nil {
		return nil, err
	}
	var response nhcResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode nhc response: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.ActiveStorms)*2)
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
		if storm.TrackCone != nil && strings.TrimSpace(storm.TrackCone.KmzFile) != "" {
			if cone, coneErr := s.sampleCone(ctx, storm, name, label, now); coneErr != nil {
				s.logger.Warn("nhc cone fetch failed", "source", s.id, "storm", storm.ID, "error", coneErr)
			} else if cone != nil {
				records = append(records, *cone)
			}
		}
		if len(records) >= maxStorms*2 {
			break
		}
	}
	return records, nil
}

func (s *Source) sampleCone(ctx context.Context, storm nhcStorm, name, label string, now time.Time) (*plugins.RawRecord, error) {
	body, err := s.fetch(ctx, storm.TrackCone.KmzFile)
	if err != nil {
		return nil, err
	}
	rings, err := parseConeGeometry(body)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"kind":        "storm-cone",
		"stormId":     storm.ID,
		"name":        name,
		"classLabel":  label,
		"advisory":    storm.TrackCone.AdvNum,
		"issuance":    storm.TrackCone.Issuance,
		"coneKind":    "track",
		"rings":       rings,
		"provider":    "nhc",
		"product":     "5-day forecast cone",
		"attribution": "NOAA National Hurricane Center",
	})
	if err != nil {
		return nil, err
	}
	record := plugins.RawRecord{
		SourcePluginID: "nhc", SourceInstanceID: s.id, OriginalID: storm.ID + ":cone",
		Domain: "weather", ObservedUTC: now, Payload: payload,
	}
	return &record, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("nhc sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("nhc snapshot", "source", s.id, "records", len(records))
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
