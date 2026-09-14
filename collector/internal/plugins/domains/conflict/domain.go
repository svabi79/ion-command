// Package conflict normalizes georeferenced organized-violence events
// into canonical point observations. Vocabulary is generic; sources own
// provider-specific fields.
package conflict

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
func (d *Domain) ID() string     { return "domain.conflict" }
func (d *Domain) Domain() string { return "conflict" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		EventID     string  `json:"eventId"`
		Title       string  `json:"title"`
		Kind        string  `json:"eventKind"`
		Country     string  `json:"country"`
		Deaths      int     `json:"deaths"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Attribution string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.EventID == "" {
		return nil, fmt.Errorf("decode conflict event")
	}
	event := events.NewEnvelope(record.OriginalID, "conflict", "conflict.event", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "conflict:event:" + raw.EventID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(14 * 24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Title
	if title == "" {
		title = raw.Kind
	}
	if title == "" {
		title = "Conflict event"
	}
	primary := raw.Kind
	if raw.Deaths > 0 {
		if primary != "" {
			primary = fmt.Sprintf("%s  //  %d killed", primary, raw.Deaths)
		} else {
			primary = fmt.Sprintf("%d killed", raw.Deaths)
		}
	}
	scale := 1.0
	if raw.Deaths >= 100 {
		scale = 1.8
	} else if raw.Deaths >= 25 {
		scale = 1.4
	}
	event.Properties = map[string]any{
		"visual.icon":        "earthquake",
		"visual.markerScale": scale,
		"visual.tint":        "0.62,0.28,0.22",
		"display.title":      title,
		"display.primary":    primary,
		"display.secondary":  firstNonEmpty(raw.Country, raw.Attribution),
	}
	measured := true
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
