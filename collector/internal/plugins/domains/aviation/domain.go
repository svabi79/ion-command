// Package aviation normalizes aircraft position fixes into canonical
// per-airframe observations.
package aviation

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

type rawAircraft struct {
	Hex      string  `json:"hex"`
	Callsign string  `json:"callsign"`
	AcType   string  `json:"acType"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
	AltFt    float64 `json:"altFt"`
	GsKt     float64 `json:"gsKt"`
	Track    float64 `json:"track"`
	OnGround bool    `json:"onGround"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.aviation" }
func (d *Domain) Domain() string { return "aviation" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawAircraft
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode aircraft record: %w", err)
	}
	if raw.Hex == "" {
		return nil, fmt.Errorf("aircraft requires hex id")
	}
	event := events.NewEnvelope(record.OriginalID, "aviation", "aviation.aircraft", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	event.EntityID = "aviation:aircraft:" + strings.ToLower(raw.Hex)
	event.Geometry = events.Point(raw.Lon, raw.Lat, raw.AltFt*0.3048)
	// Aircraft survive a few slow or rate-limited polls (sources back off up
	// to minutes under aggregator throttling) instead of blinking out.
	validUntil := record.ObservedUTC.Add(3 * time.Minute)
	event.Time.ValidUntilUTC = &validUntil
	title := strings.TrimSpace(raw.Callsign)
	if title == "" {
		title = strings.ToUpper(raw.Hex)
	}
	primary := fmt.Sprintf("FL%03.0f  //  %.0f KT", raw.AltFt/100.0, raw.GsKt)
	if raw.OnGround {
		primary = "ON GROUND"
	}
	event.Properties = map[string]any{
		"hexId":              raw.Hex,
		"altitudeFt":         raw.AltFt,
		"groundSpeedKt":      raw.GsKt,
		"trackDeg":           raw.Track,
		"visual.markerScale": 0.9,
		"visual.icon":        "aircraft",
		"display.title":      title,
		"display.primary":    primary,
	}
	// Generic kinematics: renderers orient the glyph along the compass
	// heading and dead-reckon the marker between polls.
	if !raw.OnGround && raw.GsKt > 1 {
		event.Properties["visual.headingDeg"] = raw.Track
		event.Properties["visual.speedMps"] = raw.GsKt * 0.514444
	}
	if raw.AcType != "" {
		event.Properties["display.secondary"] = raw.AcType
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}
