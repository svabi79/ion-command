// Package humanitarian normalizes country-level displacement aggregates
// and ReliefWeb disasters into canonical point annotations.
package humanitarian

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
func (d *Domain) ID() string     { return "domain.humanitarian" }
func (d *Domain) Domain() string { return "humanitarian" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(record.Payload, &kind) == nil && kind.Kind == "site" {
		return d.normalizeSite(record)
	}
	if json.Unmarshal(record.Payload, &kind) == nil && kind.Kind == "disaster" {
		return d.normalizeDisaster(record)
	}
	return d.normalizeDisplacement(record)
}

func (d *Domain) normalizeDisplacement(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		Country     string  `json:"country"`
		Name        string  `json:"name"`
		Role        string  `json:"role"`
		Population  int     `json:"population"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Attribution string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.Country == "" {
		return nil, fmt.Errorf("decode displacement record")
	}
	event := events.NewEnvelope(record.OriginalID, "humanitarian", "humanitarian.displacement", events.MessageAnnotation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "humanitarian:" + raw.Role + ":" + raw.Country
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(30 * 24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Name
	if title == "" {
		title = raw.Country
	}
	primary := raw.Role
	if raw.Population > 0 {
		primary = fmt.Sprintf("%s  //  %s", formatPopulation(raw.Population), raw.Role)
	}
	event.Properties = map[string]any{
		"visual.icon":        "station",
		"visual.markerScale": markerScale(raw.Population),
		"display.title":      title,
		"display.primary":    primary,
		"display.secondary":  raw.Attribution,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeDisaster(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		DisasterID  string  `json:"disasterId"`
		Title       string  `json:"title"`
		Status      string  `json:"status"`
		Glide       string  `json:"glide"`
		Category    string  `json:"category"`
		Country     string  `json:"country"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Attribution string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.DisasterID == "" {
		return nil, fmt.Errorf("decode disaster record")
	}
	event := events.NewEnvelope(record.OriginalID, "humanitarian", "humanitarian.disaster", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "humanitarian:disaster:" + raw.DisasterID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := strings.TrimSpace(raw.Title)
	if title == "" {
		title = raw.DisasterID
	}
	primary := strings.TrimSpace(raw.Status)
	if raw.Category != "" {
		if primary != "" {
			primary = raw.Category + "  //  " + primary
		} else {
			primary = raw.Category
		}
	}
	secondary := raw.Attribution
	if raw.Glide != "" {
		if secondary != "" {
			secondary = raw.Glide + "  //  " + secondary
		} else {
			secondary = raw.Glide
		}
	}
	event.Properties = map[string]any{
		"visual.icon":        "earthquake",
		"visual.markerScale": 1.3,
		"display.title":      title,
		"display.primary":    primary,
		"display.secondary":  secondary,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func formatPopulation(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func markerScale(n int) float64 {
	switch {
	case n >= 2000000:
		return 2.0
	case n >= 500000:
		return 1.6
	case n >= 100000:
		return 1.2
	default:
		return 0.9
	}
}

func (d *Domain) normalizeSite(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		SiteID      string  `json:"siteId"`
		Name        string  `json:"name"`
		Kind        string  `json:"siteKind"`
		Country     string  `json:"country"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Attribution string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.SiteID == "" {
		return nil, fmt.Errorf("decode humanitarian site")
	}
	event := events.NewEnvelope(record.OriginalID, "humanitarian", "humanitarian.site", events.MessageAnnotation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "humanitarian:site:" + raw.SiteID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(30 * 24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Name
	if title == "" {
		title = raw.SiteID
	}
	primary := raw.Kind
	if primary == "" {
		primary = "person of concern site"
	}
	event.Properties = map[string]any{
		"visual.icon":        "station",
		"visual.markerScale": 0.7,
		"visual.tint":        "0.72,0.62,0.38",
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
