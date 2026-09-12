package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct{}

type rawLightning struct {
	StrikeID      string  `json:"strikeId"`
	Longitude     float64 `json:"longitude"`
	Latitude      float64 `json:"latitude"`
	PeakCurrentKa float64 `json:"peakCurrentKa"`
	StationCount  int     `json:"stationCount"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.weather" }
func (d *Domain) Domain() string { return "weather" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(record.Payload, &kind) == nil {
		switch kind.Kind {
		case "observation":
			return d.normalizeObservation(record)
		case "airquality":
			return d.normalizeAirQuality(record)
		}
	}
	var raw rawLightning
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode lightning record: %w", err)
	}
	if raw.StrikeID == "" {
		return nil, fmt.Errorf("lightning strike requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.lightning", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	event.Properties = map[string]any{"peakCurrentKa": raw.PeakCurrentKa, "unit": "kA", "display.title": "Lightning strike", "visual.icon": "lightning"}
	if raw.StationCount > 0 {
		event.Properties["stationCount"] = raw.StationCount
		event.Properties["display.primary"] = fmt.Sprintf("%d stations", raw.StationCount)
	} else if raw.PeakCurrentKa != 0 {
		event.Properties["display.primary"] = fmt.Sprintf("%.1f kA", raw.PeakCurrentKa)
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeObservation(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		CellLat      float64 `json:"cellLat"`
		CellLon      float64 `json:"cellLon"`
		TemperatureC float64 `json:"temperatureC"`
		WeatherCode  int     `json:"weatherCode"`
		WindKmh      float64 `json:"windKmh"`
		Attribution  string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode weather observation: %w", err)
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.observation", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = fmt.Sprintf("weather:cell:%.1f,%.1f", raw.CellLat, raw.CellLon)
	event.Geometry = events.Point(raw.CellLon, raw.CellLat, 0)
	validUntil := record.ObservedUTC.Add(15 * time.Minute)
	event.Time.ValidUntilUTC = &validUntil
	event.Properties = map[string]any{
		"visual.icon":        "sounding",
		"visual.markerScale": 1.1,
		"display.title":      fmt.Sprintf("%.0f C", raw.TemperatureC),
		"display.primary":    fmt.Sprintf("%.0f km/h wind  //  code %d", raw.WindKmh, raw.WeatherCode),
		"display.secondary":  raw.Attribution,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeAirQuality(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		StationID   int     `json:"stationId"`
		Name        string  `json:"name"`
		Locality    string  `json:"locality"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Parameter   string  `json:"parameter"`
		Attribution string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.StationID == 0 {
		return nil, fmt.Errorf("decode air-quality station")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.airquality", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = fmt.Sprintf("weather:airquality:%d", raw.StationID)
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(2 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Name
	if title == "" {
		title = fmt.Sprintf("AQ %d", raw.StationID)
	}
	event.Properties = map[string]any{
		"visual.icon":        "sounding",
		"visual.markerScale": 0.85,
		"display.title":      title,
		"display.primary":    raw.Parameter,
		"display.secondary":  raw.Attribution,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}
