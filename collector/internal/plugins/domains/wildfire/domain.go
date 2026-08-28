// Package wildfire normalizes satellite thermal-anomaly detections (from
// NASA FIRMS today) into canonical observations. A detection is exactly
// that: a pixel where a VIIRS or MODIS band crossed the instrument's fire
// algorithm threshold. It is not a confirmed fire on the ground, and this
// domain is deliberately careful not to claim otherwise anywhere a person
// might read it — see display.title/display.primary and the "confirmed"
// property below.
package wildfire

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

// detectionValidity is how long a detection stays on the globe after its
// acquisition time. VIIRS/MODIS revisit a given point at most a handful of
// times a day; six hours keeps a detection visible through roughly one
// further overpass without accumulating fires that stopped burning long ago.
const detectionValidity = 6 * time.Hour

type rawDetection struct {
	DetectionID    string  `json:"detectionId"`
	Longitude      float64 `json:"longitude"`
	Latitude       float64 `json:"latitude"`
	Instrument     string  `json:"instrument"`
	SatelliteCode  string  `json:"satelliteCode"`
	SatelliteLabel string  `json:"satelliteLabel"`
	ConfidenceRaw  string  `json:"confidenceRaw"`
	BrightnessK    float64 `json:"brightnessK"`
	Brightness2K   float64 `json:"brightness2K"`
	FrpMw          float64 `json:"frpMw"`
	DayNight       string  `json:"dayNight"`
}

type Domain struct{}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.wildfire" }
func (d *Domain) Domain() string { return "wildfire" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawDetection
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode wildfire detection record: %w", err)
	}
	if raw.DetectionID == "" {
		return nil, fmt.Errorf("wildfire detection requires id")
	}

	event := events.NewEnvelope(record.OriginalID, "wildfire", "wildfire.detection", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "wildfire:detection:" + raw.DetectionID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)

	validUntil := record.ObservedUTC.Add(detectionValidity)
	event.Time.ValidUntilUTC = &validUntil

	dayNightLabel := dayNightLabel(raw.DayNight)
	confidenceLabel := confidenceLabel(raw.ConfidenceRaw)
	instrumentLabel := raw.Instrument
	if instrumentLabel == "" {
		instrumentLabel = "unknown sensor"
	}
	satelliteLabel := raw.SatelliteLabel
	if satelliteLabel == "" {
		satelliteLabel = raw.SatelliteCode
	}

	event.Properties = map[string]any{
		"instrument":    raw.Instrument,
		"satellite":     satelliteLabel,
		"confidenceRaw": raw.ConfidenceRaw,
		"brightnessK":   raw.BrightnessK,
		"brightness2K":  raw.Brightness2K,
		"frpMw":         raw.FrpMw,
		"dayNight":      raw.DayNight,
		// detectionType/confirmed are machine-checkable companions to the
		// display strings below, so a UI can gate on them without parsing
		// text: this is a thermal anomaly, never presented as a fire.
		"detectionType":      "thermal_anomaly",
		"confirmed":          false,
		"visual.icon":        "wildfire",
		"visual.markerScale": markerScale(raw.FrpMw),
		"display.title":      "Satellite Thermal Anomaly",
		"display.primary":    fmt.Sprintf("%s %s · %s confidence", instrumentLabel, satelliteLabel, confidenceLabel),
		"display.secondary":  fmt.Sprintf("FRP %.1f MW · %s · not a confirmed fire", raw.FrpMw, dayNightLabel),
	}
	if confidence := normalizedConfidence(raw.ConfidenceRaw); confidence != nil {
		event.Quality.Confidence = confidence
	}
	// The radiance crossing the fire-detection threshold is a real sensor
	// measurement; what it means on the ground is not. Measured therefore
	// describes the pixel reading, not a claim that a fire is confirmed —
	// that distinction is carried explicitly in "confirmed" and the display
	// text above instead of being folded into this flag.
	measured := true
	event.Quality.Measured = &measured

	return []events.Envelope{event}, nil
}

func dayNightLabel(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "D":
		return "day"
	case "N":
		return "night"
	default:
		return "unknown"
	}
}

func confidenceLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unknown"
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return raw + "%"
	}
	return strings.ToLower(raw)
}

// normalizedConfidence maps FIRMS' two confidence encodings onto one 0-1
// scale: MODIS's numeric percent divides directly; VIIRS's low/nominal/high
// categories are placed at ordinal points we chose ourselves. That ordinal
// placement is this project's own judgment call for a single sortable field,
// not a NASA-defined percentage equivalence — it must not be read as one.
func normalizedConfidence(raw string) *float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if percent, err := strconv.ParseFloat(raw, 64); err == nil {
		value := percent / 100.0
		switch {
		case value < 0:
			value = 0
		case value > 1:
			value = 1
		}
		return &value
	}
	var value float64
	switch strings.ToLower(raw) {
	case "low":
		value = 0.3
	case "nominal":
		value = 0.6
	case "high":
		value = 0.9
	default:
		return nil
	}
	return &value
}

// markerScale grows with fire radiative power on a log scale: FRP spans
// roughly 0.1 MW to several hundred MW in practice, which would make a
// linear marker size either invisible for small detections or off-screen for
// large ones. Bounds mirror the geophysics domain's earthquake scale.
func markerScale(frpMw float64) float64 {
	if frpMw < 0 {
		frpMw = 0
	}
	scale := 0.8 + math.Log10(frpMw+1)*0.6
	if scale > 3.5 {
		scale = 3.5
	}
	if scale < 0.8 {
		scale = 0.8
	}
	return scale
}
