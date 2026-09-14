// Package geophysics normalizes solid-earth events (currently earthquakes)
// into canonical observations.
package geophysics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct{}

type rawQuake struct {
	QuakeID   string  `json:"quakeId"`
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	Magnitude float64 `json:"magnitude"`
	DepthKm   float64 `json:"depthKm"`
	Place     string  `json:"place"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.geophysics" }
func (d *Domain) Domain() string { return "geophysics" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(record.Payload, &kind) == nil && kind.Kind == "event" {
		return d.normalizeEvent(record)
	}
	if json.Unmarshal(record.Payload, &kind) == nil && kind.Kind == "volcano" {
		return d.normalizeVolcano(record)
	}
	var raw rawQuake
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode earthquake record: %w", err)
	}
	if raw.QuakeID == "" {
		return nil, fmt.Errorf("earthquake requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "geophysics", "geophysics.earthquake", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	event.EntityID = "geophysics:quake:" + raw.QuakeID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	// Quakes stay on the globe for two hours; magnitude drives marker size.
	validUntil := record.ObservedUTC.Add(2 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	markerScale := 0.8 + raw.Magnitude*0.35
	if markerScale > 3.5 {
		markerScale = 3.5
	}
	if markerScale < 0.8 {
		markerScale = 0.8
	}
	event.Properties = map[string]any{
		"magnitude":          raw.Magnitude,
		"depthKm":            raw.DepthKm,
		"visual.markerScale": markerScale,
		"visual.icon":        "earthquake",
		"display.title":      fmt.Sprintf("M %.1f Earthquake", raw.Magnitude),
		"display.primary":    raw.Place,
		"display.secondary":  fmt.Sprintf("depth %.0f km", raw.DepthKm),
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeEvent(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		EventID    string  `json:"eventId"`
		Title      string  `json:"title"`
		Category   string  `json:"category"`
		AlertLevel string  `json:"alertLevel"`
		Longitude  float64 `json:"longitude"`
		Latitude   float64 `json:"latitude"`
		Provider   string  `json:"provider"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.EventID == "" {
		return nil, fmt.Errorf("decode geophysics event")
	}
	event := events.NewEnvelope(record.OriginalID, "geophysics", "geophysics.event", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geophysics:event:" + raw.EventID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	icon := "earthquake"
	if raw.Category == "wildfires" || raw.Category == "WF" {
		icon = "wildfire"
	}
	primary := raw.Category
	if raw.AlertLevel != "" {
		primary = raw.AlertLevel + "  //  " + raw.Category
	}
	event.Properties = map[string]any{
		"visual.icon":        icon,
		"visual.markerScale": 1.3,
		"display.title":      raw.Title,
		"display.primary":    primary,
		"display.secondary":  raw.Provider,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeVolcano(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		VolcanoID   string  `json:"volcanoId"`
		Name        string  `json:"name"`
		Alert       string  `json:"alert"`
		Color       string  `json:"color"`
		Region      string  `json:"region"`
		Catalog     bool    `json:"catalog"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Provider    string  `json:"provider"`
		Attribution string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.VolcanoID == "" {
		return nil, fmt.Errorf("decode geophysics volcano")
	}
	event := events.NewEnvelope(record.OriginalID, "geophysics", "geophysics.volcano", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geophysics:volcano:" + raw.VolcanoID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validFor := 36 * time.Hour
	if raw.Catalog {
		validFor = 30 * 24 * time.Hour
	}
	validUntil := record.ObservedUTC.Add(validFor)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Name
	if title == "" {
		title = "Volcano"
	}
	primary := raw.Alert
	if raw.Color != "" && raw.Color != raw.Alert {
		if primary != "" {
			primary = primary + "  //  " + raw.Color
		} else {
			primary = raw.Color
		}
	}
	if primary == "" {
		primary = "volcano"
	}
	scale := 0.9
	tint := "0.78,0.48,0.22"
	switch strings.ToUpper(strings.TrimSpace(raw.Alert)) {
	case "WARNING":
		scale, tint = 1.8, "0.82,0.22,0.14"
	case "WATCH":
		scale, tint = 1.5, "0.82,0.38,0.16"
	case "ADVISORY":
		scale, tint = 1.25, "0.82,0.55,0.18"
	case "NORMAL":
		scale, tint = 0.95, "0.55,0.48,0.32"
	}
	event.Properties = map[string]any{
		"visual.icon":        "earthquake",
		"visual.markerScale": scale,
		"visual.tint":        tint,
		"display.title":      title,
		"display.primary":    primary,
		"display.secondary":  firstNonEmpty(raw.Region, raw.Attribution, raw.Provider),
	}
	measured := !raw.Catalog
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
