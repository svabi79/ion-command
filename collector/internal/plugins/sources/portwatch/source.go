// Package portwatch polls IMF PortWatch chokepoint locations and the
// latest daily transit counts from the public ArcGIS FeatureServer.
package portwatch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultLocations = "https://services9.arcgis.com/weJ1QsnbMYJlCHdG/arcgis/rest/services/PortWatch_chokepoints_database/FeatureServer/0/query?where=1%3D1&outFields=*&outSR=4326&f=json"
	defaultDaily     = "https://services9.arcgis.com/weJ1QsnbMYJlCHdG/arcgis/rest/services/Daily_Chokepoints_Data/FeatureServer/0/query?where=1%3D1&outFields=*&orderByFields=date%20DESC&resultRecordCount=80&f=json"
	pollDefault      = 6 * time.Hour
	pollFloor        = time.Hour
	maxPoints        = 40
)

type Source struct {
	id           string
	locationsURL string
	dailyURL     string
	interval     time.Duration
	client       *http.Client
	logger       *slog.Logger
	fetch        func(ctx context.Context, url string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "maritime.portwatch" {
		return nil, fmt.Errorf("unsupported portwatch source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("portwatch poll interval below one hour (got %s)", interval)
	}
	source := &Source{
		id: sourceConfig.ID, locationsURL: defaultLocations, dailyURL: defaultDaily,
		interval: interval, client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	if sourceConfig.Broker != "" {
		source.locationsURL = sourceConfig.Broker
	}
	if sourceConfig.Topic != "" {
		source.dailyURL = sourceConfig.Topic
	}
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, rawURL, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "maritime.portwatch" }

type arcGIS struct {
	Features []struct {
		Attributes map[string]any `json:"attributes"`
		Geometry   *struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"geometry"`
	} `json:"features"`
}

func attrString(attrs map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := attrs[key]; ok {
			switch typed := value.(type) {
			case string:
				if typed != "" {
					return typed
				}
			case float64:
				return fmt.Sprintf("%.0f", typed)
			}
		}
	}
	return ""
}

func attrFloat(attrs map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := attrs[key]; ok {
			if number, ok := value.(float64); ok {
				return number, true
			}
		}
	}
	return 0, false
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	locBody, err := s.fetch(ctx, s.locationsURL)
	if err != nil {
		return nil, err
	}
	var locations arcGIS
	if err := json.Unmarshal(locBody, &locations); err != nil {
		return nil, fmt.Errorf("decode portwatch locations: %w", err)
	}
	dailyByID := map[string]map[string]any{}
	if dailyBody, err := s.fetch(ctx, s.dailyURL); err == nil {
		var daily arcGIS
		if json.Unmarshal(dailyBody, &daily) == nil {
			for _, feature := range daily.Features {
				id := attrString(feature.Attributes, "portid", "portId", "OBJECTID")
				if id != "" {
					if _, exists := dailyByID[id]; !exists {
						dailyByID[id] = feature.Attributes
					}
				}
			}
		}
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(locations.Features))
	for _, feature := range locations.Features {
		lon, lat := 0.0, 0.0
		if feature.Geometry != nil {
			lon, lat = feature.Geometry.X, feature.Geometry.Y
		} else {
			var ok1, ok2 bool
			lon, ok1 = attrFloat(feature.Attributes, "lon", "longitude")
			lat, ok2 = attrFloat(feature.Attributes, "lat", "latitude")
			if !ok1 || !ok2 {
				continue
			}
		}
		id := attrString(feature.Attributes, "portid", "portId", "chokepointid")
		name := attrString(feature.Attributes, "portname", "portName", "name")
		if id == "" {
			id = name
		}
		if id == "" {
			continue
		}
		transits := 0.0
		if daily, ok := dailyByID[id]; ok {
			if value, ok := attrFloat(daily, "n_total", "n", "n_vessels"); ok {
				transits = value
			}
			if name == "" {
				name = attrString(daily, "portname", "portName")
			}
		}
		payload, err := json.Marshal(map[string]any{
			"kind":      "chokepoint",
			"pointId":   id,
			"name":      name,
			"latitude":  lat,
			"longitude": lon,
			"transits":  transits,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "portwatch", SourceInstanceID: s.id, OriginalID: "pw-" + id,
			Domain: "maritime", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxPoints {
			break
		}
	}
	records = append(records, s.sampleDisruptions(ctx, now)...)
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("portwatch sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("portwatch snapshot", "source", s.id, "chokepoints", len(records))
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
