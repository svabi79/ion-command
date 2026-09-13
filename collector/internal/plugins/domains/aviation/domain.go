// Package aviation normalizes aircraft position fixes into canonical
// per-airframe observations.
package aviation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type rememberedEmergency struct {
	alarm string
	until time.Time
}

type Domain struct {
	mu          sync.Mutex
	emergencies map[string]rememberedEmergency
}

type rawAircraft struct {
	Hex               string  `json:"hex"`
	Callsign          string  `json:"callsign"`
	AcType            string  `json:"acType"`
	Registration      string  `json:"registration"`
	Kind              string  `json:"kind"` // aircraft | helicopter | glider | balloon | drone
	Squawk            string  `json:"squawk"`
	OriginCountry     string  `json:"originCountry"`
	BaroRateFpm       float64 `json:"baroRateFpm"`
	Lat               float64 `json:"lat"`
	Lon               float64 `json:"lon"`
	AltFt             float64 `json:"altFt"`
	GsKt              float64 `json:"gsKt"`
	Track             float64 `json:"track"`
	OnGround          bool    `json:"onGround"`
	ValidSeconds      int     `json:"validSeconds"`
	LastContactAgeSec int     `json:"lastContactAgeSec"`
	// Filed route, when the source resolved one for the callsign.
	RouteOriginCode string `json:"routeOriginCode"`
	RouteOriginCity string `json:"routeOriginCity"`
	RouteDestCode   string `json:"routeDestCode"`
	RouteDestCity   string `json:"routeDestCity"`
}

// emergencySquawks flag hijack (7500), radio failure (7600), and general
// emergency (7700).
var emergencySquawks = map[string]string{
	"7500": "HIJACK",
	"7600": "RADIO FAILURE",
	"7700": "EMERGENCY",
}

const (
	emergencyMemory   = 2 * time.Hour
	emergencyValidFor = 2 * time.Hour
	maxRemembered     = 4096
)

func New() *Domain {
	return &Domain{emergencies: make(map[string]rememberedEmergency)}
}
func (d *Domain) ID() string     { return "domain.aviation" }
func (d *Domain) Domain() string { return "aviation" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(record.Payload, &kind) == nil && kind.Kind == "interference" {
		return d.normalizeInterference(record)
	}
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
	hex := strings.ToLower(raw.Hex)
	alarm, isEmergency := d.rememberEmergency(hex, raw.Squawk, record.ObservedUTC)
	if isEmergency && validFor < emergencyValidFor {
		validFor = emergencyValidFor
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
	airKind := raw.Kind
	if airKind == "" {
		airKind = "aircraft"
	}
	if airKind != "aircraft" {
		details = append(details, strings.ToUpper(airKind))
	}
	event.Properties = map[string]any{
		"hexId":              raw.Hex,
		"altitudeFt":         raw.AltFt,
		"groundSpeedKt":      raw.GsKt,
		"trackDeg":           raw.Track,
		"onGround":           raw.OnGround,
		"visual.markerScale": 0.9,
		"visual.icon":        airKind,
		// True altitude is visually imperceptible at globe scale (10 km on
		// a 6371 km sphere); render it exaggerated, honest numbers stay in
		// the tooltip.
		"visual.altitudeScale": 12,
		"display.title":        title,
		"display.primary":      primary,
	}
	if isEmergency {
		event.Properties["display.title"] = title + "  //  " + alarm
		event.Properties["visual.tint"] = "1.0,0.15,0.1"
		event.Properties["visual.markerScale"] = 2.4
		// Sticky across later null-squawk snapshots (typical of OpenSky)
		// and across on-ground reports — 7500/7600/7700 stay visible.
		event.Properties["visual.emergency"] = alarm
	}
	// Generic kinematics: renderers orient the glyph along the compass
	// heading and dead-reckon the marker between polls.
	if !raw.OnGround && raw.GsKt > 1 {
		event.Properties["visual.headingDeg"] = raw.Track
		event.Properties["visual.speedMps"] = raw.GsKt * 0.514444
	}
	if raw.LastContactAgeSec > 0 {
		event.Properties["visual.lastContactAgeSec"] = raw.LastContactAgeSec
	}
	if len(details) > 0 {
		event.Properties["display.secondary"] = strings.Join(details, "  //  ")
	}
	// Filed route as its own tooltip line, human readable: "CDG Paris > TUN
	// Tunis". Codes alone still beat nothing when a city is missing.
	if raw.RouteOriginCode != "" && raw.RouteDestCode != "" {
		origin := strings.TrimSpace(raw.RouteOriginCode + " " + raw.RouteOriginCity)
		destination := strings.TrimSpace(raw.RouteDestCode + " " + raw.RouteDestCity)
		event.Properties["display.tertiary"] = origin + "  >  " + destination
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeInterference(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		H3            string  `json:"h3"`
		Latitude      float64 `json:"latitude"`
		Longitude     float64 `json:"longitude"`
		Level         string  `json:"level"`
		Pct           float64 `json:"pct"`
		TotalAircraft int     `json:"totalAircraft"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.H3 == "" {
		return nil, fmt.Errorf("decode interference cell")
	}
	event := events.NewEnvelope(record.OriginalID, "aviation", "aviation.interference", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "aviation:interference:" + raw.H3
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(36 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	tint := "1.0,0.75,0.15"
	if raw.Level == "high" {
		tint = "1.0,0.2,0.1"
	}
	event.Properties = map[string]any{
		"visual.icon":        "drone",
		"visual.markerScale": 1.2,
		"visual.tint":        tint,
		"display.title":      strings.ToUpper(raw.Level) + " GNSS interference",
		"display.primary":    fmt.Sprintf("%.1f%% low-accuracy reports", raw.Pct),
		"display.secondary":  fmt.Sprintf("%d aircraft", raw.TotalAircraft),
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) rememberEmergency(hex, squawk string, observed time.Time) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.emergencies == nil {
		d.emergencies = make(map[string]rememberedEmergency)
	}
	if alarm, ok := emergencySquawks[squawk]; ok {
		if len(d.emergencies) >= maxRemembered {
			for existing, mem := range d.emergencies {
				if mem.until.Before(observed) {
					delete(d.emergencies, existing)
				}
			}
			if len(d.emergencies) >= maxRemembered {
				for existing := range d.emergencies {
					delete(d.emergencies, existing)
					break
				}
			}
		}
		d.emergencies[hex] = rememberedEmergency{alarm: alarm, until: observed.Add(emergencyMemory)}
		return alarm, true
	}
	if mem, ok := d.emergencies[hex]; ok {
		if !observed.After(mem.until) {
			return mem.alarm, true
		}
		delete(d.emergencies, hex)
	}
	return "", false
}
