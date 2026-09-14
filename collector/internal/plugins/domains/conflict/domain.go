// Package conflict normalizes operator-keyed ACLED events into canonical
// point observations. Fatality counts are passed through as reported;
// this domain does not invent a severity scale.
package conflict

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

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.conflict" }
func (d *Domain) Domain() string { return "conflict" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		EventId      string  `json:"eventId"`
		EventType    string  `json:"eventType"`
		SubEventType string  `json:"subEventType"`
		Country      string  `json:"country"`
		Location     string  `json:"location"`
		Latitude     float64 `json:"latitude"`
		Longitude    float64 `json:"longitude"`
		Fatalities   int     `json:"fatalities"`
		Attribution  string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || strings.TrimSpace(raw.EventId) == "" {
		return nil, fmt.Errorf("decode conflict event")
	}
	id := strings.TrimSpace(raw.EventId)
	event := events.NewEnvelope(record.OriginalID, "conflict", "conflict.event", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "conflict:event:" + id
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := strings.TrimSpace(raw.Location)
	if title == "" {
		title = strings.TrimSpace(raw.Country)
	} else if raw.Country != "" && !strings.EqualFold(title, raw.Country) {
		title = title + ", " + raw.Country
	}
	if title == "" {
		title = id
	}
	primary := strings.TrimSpace(raw.EventType)
	if raw.SubEventType != "" {
		if primary != "" {
			primary = raw.SubEventType + "  //  " + primary
		} else {
			primary = raw.SubEventType
		}
	}
	secondary := raw.Attribution
	if raw.Fatalities > 0 {
		reported := fmt.Sprintf("%d reported fatalities", raw.Fatalities)
		if secondary != "" {
			secondary = reported + "  //  " + secondary
		} else {
			secondary = reported
		}
	}
	event.Properties = map[string]any{
		"visual.icon":        "station",
		"visual.markerScale": 1.0,
		"display.title":      title,
		"display.primary":    primary,
		"display.secondary":  secondary,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}
