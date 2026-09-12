// Package space normalizes orbital-launch observations (pad, vehicle,
// payload, status) into canonical point messages.
package space

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

type rawLaunch struct {
	LaunchID    string  `json:"launchId"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Net         string  `json:"net"`
	PadName     string  `json:"padName"`
	PadLocation string  `json:"padLocation"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Rocket      string  `json:"rocket"`
	Mission     string  `json:"mission"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.space" }
func (d *Domain) Domain() string { return "space" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawLaunch
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode launch record: %w", err)
	}
	if raw.LaunchID == "" {
		return nil, fmt.Errorf("launch requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "space", "space.launch", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "space:launch:" + raw.LaunchID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(36 * time.Hour)
	if netTime, err := time.Parse(time.RFC3339, raw.Net); err == nil {
		until := netTime.Add(6 * time.Hour)
		if until.After(validUntil) {
			validUntil = until
		}
	}
	event.Time.ValidUntilUTC = &validUntil
	title := strings.TrimSpace(raw.Name)
	if title == "" {
		title = raw.Rocket
	}
	primary := raw.Status
	if raw.Net != "" {
		if netTime, err := time.Parse(time.RFC3339, raw.Net); err == nil {
			primary = netTime.UTC().Format("02 Jan 15:04Z") + "  //  " + raw.Status
		}
	}
	event.Properties = map[string]any{
		"visual.icon":        "satellite",
		"visual.markerScale": 1.4,
		"display.title":      title,
		"display.primary":    primary,
		"display.secondary":  strings.TrimSpace(raw.PadName + "  //  " + raw.PadLocation),
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}
