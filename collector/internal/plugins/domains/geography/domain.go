// Package geography normalizes static cartographic labels: named regions
// and submarine-cable geometry.
package geography

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct{}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.geography" }
func (d *Domain) Domain() string { return "geography" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(record.Payload, &kind); err != nil {
		return nil, fmt.Errorf("decode geography record: %w", err)
	}
	switch kind.Kind {
	case "region":
		return d.region(record)
	case "cable":
		return d.cable(record)
	case "landing":
		return d.landing(record)
	default:
		return nil, fmt.Errorf("geography record has unknown kind %q", kind.Kind)
	}
}

func (d *Domain) region(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		Name       string  `json:"name"`
		RegionKind string  `json:"regionKind"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.Name == "" {
		return nil, fmt.Errorf("geography region requires a name")
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.region", events.MessageAnnotation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:region:" + raw.Name
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(30 * 24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	icon := "station"
	if raw.RegionKind == "marine" {
		icon = "signal"
	}
	event.Properties = map[string]any{
		"visual.icon":        icon,
		"visual.markerScale": 0.7,
		"display.title":      raw.Name,
		"display.primary":    raw.RegionKind,
	}
	return []events.Envelope{event}, nil
}

func (d *Domain) cable(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		CableID string  `json:"cableId"`
		Name    string  `json:"name"`
		FromLon float64 `json:"fromLon"`
		FromLat float64 `json:"fromLat"`
		ToLon   float64 `json:"toLon"`
		ToLat   float64 `json:"toLat"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.CableID == "" {
		return nil, fmt.Errorf("geography cable requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.cable", events.MessageRelationship,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:cable:" + raw.CableID
	event.Geometry = events.GreatCircle(raw.FromLon, raw.FromLat, raw.ToLon, raw.ToLat)
	validUntil := record.ObservedUTC.Add(30 * 24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	event.Properties = map[string]any{
		"visual.paletteIndex": 7,
		"display.title":       raw.Name,
		"display.primary":     "submarine cable",
	}
	return []events.Envelope{event}, nil
}

func (d *Domain) landing(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		LandingID string  `json:"landingId"`
		Name      string  `json:"name"`
		Longitude float64 `json:"longitude"`
		Latitude  float64 `json:"latitude"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.LandingID == "" {
		return nil, fmt.Errorf("geography landing requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.landing", events.MessageAnnotation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:landing:" + raw.LandingID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(30 * 24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	event.Properties = map[string]any{
		"visual.icon":        "signal",
		"visual.markerScale": 0.55,
		"display.title":      raw.Name,
		"display.primary":    "cable landing",
	}
	return []events.Envelope{event}, nil
}
