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
	// Look angles from the operator's station, present only when the source
	// has one configured. Elevation is negative below the horizon.
	AzDeg   *float64 `json:"azDeg,omitempty"`
	ElDeg   *float64 `json:"elDeg,omitempty"`
	RangeKm *float64 `json:"rangeKm,omitempty"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.orbital" }
func (d *Domain) Domain() string { return "orbital" }

type rawPass struct {
	Kind      string  `json:"kind"`
	SatID     string  `json:"satId"`
	Name      string  `json:"name"`
	AosUTC    string  `json:"aosUtc"`
	TcaUTC    string  `json:"tcaUtc"`
	LosUTC    string  `json:"losUtc"`
	PeakElDeg float64 `json:"peakElDeg"`
	AosAzDeg  float64 `json:"aosAzDeg"`
	LosAzDeg  float64 `json:"losAzDeg"`
	DurationS float64 `json:"durationS"`
}

// compass turns a bearing into the eight-point name an operator actually
// points an antenna by.
func compass(degrees float64) string {
	points := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	index := int((degrees+22.5)/45.0) % 8
	if index < 0 {
		index += 8
	}
	return points[index]
}

func (d *Domain) normalizePass(record plugins.RawRecord, raw rawPass) ([]events.Envelope, error) {
	if raw.SatID == "" {
		return nil, fmt.Errorf("satellite pass requires id")
	}
	acquisition, err := time.Parse(time.RFC3339, raw.AosUTC)
	if err != nil {
		return nil, fmt.Errorf("decode pass acquisition time: %w", err)
	}
	loss, err := time.Parse(time.RFC3339, raw.LosUTC)
	if err != nil {
		return nil, fmt.Errorf("decode pass loss time: %w", err)
	}
	event := events.NewEnvelope(record.OriginalID, "orbital", "orbital.pass", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	// Keyed by satellite, not by pass: a re-prediction of the same satellite
	// supersedes the previous answer rather than accumulating.
	event.EntityID = "orbital:pass:" + raw.SatID
	// A pass is not a place, so it carries no geometry. It expires when the
	// satellite sets, which is also when the prediction stops being useful.
	event.Time.ValidUntilUTC = &loss
	event.Properties = map[string]any{
		"noradId":           raw.SatID,
		"aosUtc":            acquisition.UTC().Format(time.RFC3339),
		"tcaUtc":            raw.TcaUTC,
		"losUtc":            loss.UTC().Format(time.RFC3339),
		"peakElevationDeg":  raw.PeakElDeg,
		"aosAzimuthDeg":     raw.AosAzDeg,
		"losAzimuthDeg":     raw.LosAzDeg,
		"durationS":         raw.DurationS,
		"display.title":     raw.Name,
		"display.primary":   fmt.Sprintf("AOS %s  %s -> %s", acquisition.UTC().Format("15:04Z"), compass(raw.AosAzDeg), compass(raw.LosAzDeg)),
		"display.secondary": fmt.Sprintf("peak %.0f deg  //  %.0f min", raw.PeakElDeg, raw.DurationS/60.0),
	}
	modeled := false
	event.Quality.Measured = &modeled
	return []events.Envelope{event}, nil
}

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(record.Payload, &kind); err == nil && kind.Kind == "pass" {
		var pass rawPass
		if err := json.Unmarshal(record.Payload, &pass); err != nil {
			return nil, fmt.Errorf("decode satellite pass: %w", err)
		}
		return d.normalizePass(record, pass)
	}
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
		"visual.icon":        "satellite",
		"display.title":      raw.Name,
		"display.primary":    fmt.Sprintf("alt %.0f km", raw.AltKm),
	}
	if raw.ElDeg != nil && raw.AzDeg != nil {
		event.Properties["elevationDeg"] = *raw.ElDeg
		event.Properties["azimuthDeg"] = *raw.AzDeg
		if raw.RangeKm != nil {
			event.Properties["rangeKm"] = *raw.RangeKm
		}
		// Above the horizon is the whole question for a station operator, so
		// it is a property rather than something every consumer re-derives
		// from a sign test.
		event.Properties["aboveHorizon"] = *raw.ElDeg > 0
		if *raw.ElDeg > 0 {
			event.Properties["display.secondary"] = fmt.Sprintf("EL %.0f  AZ %.0f", *raw.ElDeg, *raw.AzDeg)
			// Visible passes are what the operator is looking for; make them
			// stand out from the hundreds of satellites below the horizon.
			event.Properties["visual.markerScale"] = 2.5
			event.Properties["visual.tint"] = "0.2,1.0,0.6"
		}
	}
	modeled := false
	event.Quality.Measured = &modeled
	return []events.Envelope{event}, nil
}
