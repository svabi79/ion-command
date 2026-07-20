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
	Hex           string  `json:"hex"`
	Callsign      string  `json:"callsign"`
	AcType        string  `json:"acType"`
	Registration  string  `json:"registration"`
	Kind          string  `json:"kind"` // aircraft | helicopter | glider | balloon | drone
	Squawk        string  `json:"squawk"`
	OriginCountry string  `json:"originCountry"`
	BaroRateFpm   float64 `json:"baroRateFpm"`
	Lat           float64 `json:"lat"`
	Lon           float64 `json:"lon"`
	AltFt         float64 `json:"altFt"`
	GsKt          float64 `json:"gsKt"`
	Track         float64 `json:"track"`
	OnGround      bool    `json:"onGround"`
	ValidSeconds  int     `json:"validSeconds"`
}

// emergencySquawks flag hijack (7500), radio failure (7600), and general
// emergency (7700).
var emergencySquawks = map[string]string{
	"7500": "HIJACK",
	"7600": "RADIO FAILURE",
	"7700": "EMERGENCY",
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
	// Aircraft survive a few slow or rate-limited polls; slow global
	// snapshot sources declare their own validity horizon.
	validFor := 3 * time.Minute
	if raw.ValidSeconds > 0 {
		validFor = time.Duration(raw.ValidSeconds) * time.Second
	}
	validUntil := record.ObservedUTC.Add(validFor)
	event.Time.ValidUntilUTC = &validUntil
	title := strings.TrimSpace(raw.Callsign)
	if title == "" {
		title = strings.ToUpper(raw.Hex)
	}
	primary := fmt.Sprintf("FL%03.0f  //  %.0f KT", raw.AltFt/100.0, raw.GsKt)
	if raw.BaroRateFpm > 100 {
		primary += fmt.Sprintf("  //  CLB %.0f FPM", raw.BaroRateFpm)
	} else if raw.BaroRateFpm < -100 {
		primary += fmt.Sprintf("  //  DES %.0f FPM", -raw.BaroRateFpm)
	}
	if raw.OnGround {
		primary = "ON GROUND"
	}
	// Secondary tooltip line: whatever identity details the source knew.
	var details []string
	if raw.AcType != "" {
		details = append(details, raw.AcType)
	}
	if raw.Registration != "" {
		details = append(details, raw.Registration)
	}
	if raw.Squawk != "" {
		details = append(details, "SQ "+raw.Squawk)
	}
	if raw.OriginCountry != "" {
		details = append(details, raw.OriginCountry)
	}
	kind := raw.Kind
	if kind == "" {
		kind = "aircraft"
	}
	if kind != "aircraft" {
		details = append(details, strings.ToUpper(kind))
	}
	event.Properties = map[string]any{
		"hexId":              raw.Hex,
		"altitudeFt":         raw.AltFt,
		"groundSpeedKt":      raw.GsKt,
		"trackDeg":           raw.Track,
		"visual.markerScale": 0.9,
		"visual.icon":        kind,
		// True altitude is visually imperceptible at globe scale (10 km on
		// a 6371 km sphere); render it exaggerated, honest numbers stay in
		// the tooltip.
		"visual.altitudeScale": 12,
		"display.title":        title,
		"display.primary":      primary,
	}
	if alarm, isEmergency := emergencySquawks[raw.Squawk]; isEmergency && !raw.OnGround {
		event.Properties["display.title"] = title + "  //  " + alarm
		event.Properties["visual.tint"] = "1.0,0.15,0.1"
		event.Properties["visual.markerScale"] = 2.0
		// Explicit flag so the renderer keeps the alarm sticky when a later
		// source (e.g. OpenSky with a null squawk) re-reports the same hex
		// without emergency context.
		event.Properties["visual.emergency"] = alarm
	}
	// Generic kinematics: renderers orient the glyph along the compass
	// heading and dead-reckon the marker between polls.
	if !raw.OnGround && raw.GsKt > 1 {
		event.Properties["visual.headingDeg"] = raw.Track
		event.Properties["visual.speedMps"] = raw.GsKt * 0.514444
	}
	if len(details) > 0 {
		event.Properties["display.secondary"] = strings.Join(details, "  //  ")
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}
