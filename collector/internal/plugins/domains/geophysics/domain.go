// Package geophysics normalizes solid-earth events (currently earthquakes)
// into canonical observations.
package geophysics

import (
	"context"
	"encoding/json"
	"fmt"
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
