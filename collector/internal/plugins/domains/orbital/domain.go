// Package orbital normalizes satellite position fixes into canonical
// per-object observations.
package orbital

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct{}

type rawPosition struct {
	SatID     string  `json:"satId"`
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	AltKm     float64 `json:"altKm"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.orbital" }
func (d *Domain) Domain() string { return "orbital" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawPosition
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode satellite position: %w", err)
	}
	if raw.SatID == "" {
		return nil, fmt.Errorf("satellite position requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "orbital", "orbital.position", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	event.EntityID = "orbital:sat:" + raw.SatID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, raw.AltKm*1000.0)
	// If the feed stops, the marker leaves the globe within a minute instead
	// of freezing mid-orbit.
	validUntil := record.ObservedUTC.Add(time.Minute)
	event.Time.ValidUntilUTC = &validUntil
	event.Properties = map[string]any{
		"noradId":            raw.SatID,
		"altKm":              raw.AltKm,
		"visual.markerScale": 1.5,
		"display.title":      raw.Name,
		"display.primary":    fmt.Sprintf("alt %.0f km", raw.AltKm),
	}
	modeled := false
	event.Quality.Measured = &modeled
	return []events.Envelope{event}, nil
}
