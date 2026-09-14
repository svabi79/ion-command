package weather

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

type rawLightning struct {
	StrikeID      string  `json:"strikeId"`
	Longitude     float64 `json:"longitude"`
	Latitude      float64 `json:"latitude"`
	PeakCurrentKa float64 `json:"peakCurrentKa"`
	StationCount  int     `json:"stationCount"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.weather" }
func (d *Domain) Domain() string { return "weather" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(record.Payload, &kind) == nil {
		switch kind.Kind {
		case "observation":
			return d.normalizeObservation(record)
		case "airquality":
			return d.normalizeAirQuality(record)
		case "storm":
			return d.normalizeStorm(record)
		case "storm-cone":
			return d.normalizeStormCone(record)
		case "alert":
			return d.normalizeAlert(record)
		case "outlook":
			return d.normalizeOutlook(record)
		}
	}
	var raw rawLightning
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode lightning record: %w", err)
	}
	if raw.StrikeID == "" {
		return nil, fmt.Errorf("lightning strike requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.lightning", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	event.Properties = map[string]any{"peakCurrentKa": raw.PeakCurrentKa, "unit": "kA", "display.title": "Lightning strike", "visual.icon": "lightning"}
	if raw.StationCount > 0 {
		event.Properties["stationCount"] = raw.StationCount
		event.Properties["display.primary"] = fmt.Sprintf("%d stations", raw.StationCount)
	} else if raw.PeakCurrentKa != 0 {
		event.Properties["display.primary"] = fmt.Sprintf("%.1f kA", raw.PeakCurrentKa)
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeObservation(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		CellLat      float64 `json:"cellLat"`
		CellLon      float64 `json:"cellLon"`
		TemperatureC float64 `json:"temperatureC"`
		WeatherCode  int     `json:"weatherCode"`
		WindKmh      float64 `json:"windKmh"`
		Attribution  string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode weather observation: %w", err)
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.observation", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = fmt.Sprintf("weather:cell:%.1f,%.1f", raw.CellLat, raw.CellLon)
	event.Geometry = events.Point(raw.CellLon, raw.CellLat, 0)
	validUntil := record.ObservedUTC.Add(15 * time.Minute)
	event.Time.ValidUntilUTC = &validUntil
	event.Properties = map[string]any{
		"visual.icon":        "sounding",
		"visual.markerScale": 1.1,
		"display.title":      fmt.Sprintf("%.0f C", raw.TemperatureC),
		"display.primary":    fmt.Sprintf("%.0f km/h wind  //  code %d", raw.WindKmh, raw.WeatherCode),
		"display.secondary":  raw.Attribution,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeAirQuality(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		StationID   int     `json:"stationId"`
		Name        string  `json:"name"`
		Locality    string  `json:"locality"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Parameter   string  `json:"parameter"`
		Attribution string  `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.StationID == 0 {
		return nil, fmt.Errorf("decode air-quality station")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.airquality", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = fmt.Sprintf("weather:airquality:%d", raw.StationID)
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(2 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Name
	if title == "" {
		title = fmt.Sprintf("AQ %d", raw.StationID)
	}
	event.Properties = map[string]any{
		"visual.icon":        "sounding",
		"visual.markerScale": 0.85,
		"display.title":      title,
		"display.primary":    raw.Parameter,
		"display.secondary":  raw.Attribution,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeStorm(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		StormID     string  `json:"stormId"`
		Name        string  `json:"name"`
		ClassLabel  string  `json:"classLabel"`
		IntensityKt float64 `json:"intensityKt"`
		PressureHpa float64 `json:"pressureHpa"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		Provider    string  `json:"provider"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.StormID == "" {
		return nil, fmt.Errorf("decode weather storm")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.storm", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "weather:storm:" + raw.StormID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(12 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Name
	if raw.ClassLabel != "" {
		title = raw.ClassLabel + " " + raw.Name
	}
	primary := fmt.Sprintf("%.0f kt", raw.IntensityKt)
	if raw.PressureHpa > 0 {
		primary = fmt.Sprintf("%.0f kt  //  %.0f hPa", raw.IntensityKt, raw.PressureHpa)
	}
	scale := 1.2 + raw.IntensityKt/80
	if scale > 2.6 {
		scale = 2.6
	}
	event.Properties = map[string]any{
		"visual.icon":        "sounding",
		"visual.markerScale": scale,
		"display.title":      title,
		"display.primary":    primary,
		"display.secondary":  raw.Provider,
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeStormCone(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		StormID     string        `json:"stormId"`
		Name        string        `json:"name"`
		ClassLabel  string        `json:"classLabel"`
		Advisory    string        `json:"advisory"`
		Issuance    string        `json:"issuance"`
		ConeKind    string        `json:"coneKind"`
		Rings       [][][]float64 `json:"rings"`
		Provider    string        `json:"provider"`
		Product     string        `json:"product"`
		Attribution string        `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.StormID == "" || len(raw.Rings) == 0 {
		return nil, fmt.Errorf("decode weather storm cone")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.storm.cone", events.MessageArea,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "weather:storm:" + raw.StormID
	event.Geometry = events.Polygon(raw.Rings)
	validUntil := record.ObservedUTC.Add(18 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Name
	if raw.ClassLabel != "" {
		title = raw.ClassLabel + " " + raw.Name
	}
	if title == "" {
		title = raw.StormID
	}
	primary := "5-day forecast cone"
	if raw.Product != "" {
		primary = raw.Product
	}
	if raw.Advisory != "" {
		primary = primary + "  //  adv " + raw.Advisory
	}
	secondary := raw.Attribution
	if secondary == "" {
		secondary = raw.Provider
	}
	event.Properties = map[string]any{
		"visual.color":      "1.0,0.55,0.10",
		"visual.opacity":    0.22,
		"display.title":     title,
		"display.primary":   primary,
		"display.secondary": secondary,
	}
	if raw.ConeKind != "" {
		event.Properties["coneKind"] = raw.ConeKind
	}
	event.Relationships = []events.RelationshipRef{{Type: "forecastFor", TargetID: "weather:storm:" + raw.StormID}}
	measured := false
	event.Quality.Measured = &measured
	event.Quality.Classification = "modelled"
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeAlert(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		AlertID     string          `json:"alertId"`
		Event       string          `json:"event"`
		Severity    string          `json:"severity"`
		Headline    string          `json:"headline"`
		Area        string          `json:"area"`
		Expires     string          `json:"expires"`
		Rings       [][][]float64   `json:"rings"`
		Polygons    [][][][]float64 `json:"polygons"`
		Provider    string          `json:"provider"`
		Attribution string          `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.AlertID == "" {
		return nil, fmt.Errorf("decode weather alert")
	}
	geom, ok := areaGeometry(raw.Rings, raw.Polygons)
	if !ok {
		return nil, fmt.Errorf("weather alert requires polygon")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.alert", events.MessageArea,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "weather:alert:" + raw.AlertID
	event.Geometry = geom
	validUntil := record.ObservedUTC.Add(6 * time.Hour)
	if expires, err := time.Parse(time.RFC3339, raw.Expires); err == nil && expires.After(record.ObservedUTC) {
		validUntil = expires.UTC()
	}
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Event
	if title == "" {
		title = raw.Headline
	}
	if title == "" {
		title = "Weather alert"
	}
	primary := raw.Severity
	if raw.Area != "" {
		if primary != "" {
			primary = primary + "  //  " + raw.Area
		} else {
			primary = raw.Area
		}
	}
	color := alertColor(raw.Severity)
	event.Properties = map[string]any{
		"visual.color":      color,
		"visual.opacity":    0.18,
		"display.title":     title,
		"display.primary":   primary,
		"display.secondary": firstNonEmpty(raw.Attribution, raw.Provider),
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}

func (d *Domain) normalizeOutlook(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		OutlookID   string          `json:"outlookId"`
		Label       string          `json:"label"`
		Day         string          `json:"day"`
		Expires     string          `json:"expires"`
		Rings       [][][]float64   `json:"rings"`
		Polygons    [][][][]float64 `json:"polygons"`
		Provider    string          `json:"provider"`
		Attribution string          `json:"attribution"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.OutlookID == "" {
		return nil, fmt.Errorf("decode weather outlook")
	}
	geom, ok := areaGeometry(raw.Rings, raw.Polygons)
	if !ok {
		return nil, fmt.Errorf("weather outlook requires polygon")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.outlook", events.MessageArea,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "weather:outlook:" + raw.OutlookID
	event.Geometry = geom
	validUntil := record.ObservedUTC.Add(24 * time.Hour)
	if expires, err := time.Parse(time.RFC3339, raw.Expires); err == nil && expires.After(record.ObservedUTC) {
		validUntil = expires.UTC()
	}
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Label
	if title == "" {
		title = "Convective outlook"
	}
	event.Properties = map[string]any{
		"visual.color":      outlookColor(raw.Label),
		"visual.opacity":    0.16,
		"display.title":     title,
		"display.primary":   firstNonEmpty(raw.Day, "outlook"),
		"display.secondary": firstNonEmpty(raw.Attribution, raw.Provider),
	}
	measured := false
	event.Quality.Measured = &measured
	event.Quality.Classification = "modelled"
	return []events.Envelope{event}, nil
}

func areaGeometry(rings [][][]float64, polygons [][][][]float64) (events.Geometry, bool) {
	if len(polygons) > 1 {
		return events.MultiPolygon(polygons), true
	}
	if len(polygons) == 1 {
		return events.Polygon(polygons[0]), true
	}
	if len(rings) > 0 {
		return events.Polygon(rings), true
	}
	return events.Geometry{}, false
}

func alertColor(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "extreme":
		return "0.72,0.22,0.16"
	case "severe":
		return "0.78,0.38,0.16"
	case "moderate":
		return "0.78,0.55,0.22"
	case "minor":
		return "0.70,0.62,0.28"
	default:
		return "0.72,0.50,0.24"
	}
}

func outlookColor(label string) string {
	lower := strings.ToLower(label)
	switch {
	case strings.Contains(lower, "high"):
		return "0.70,0.22,0.18"
	case strings.Contains(lower, "moderate") || strings.Contains(lower, "mdt"):
		return "0.78,0.40,0.16"
	case strings.Contains(lower, "enhanced") || strings.Contains(lower, "enh"):
		return "0.78,0.52,0.18"
	case strings.Contains(lower, "slight") || strings.Contains(lower, "slgt"):
		return "0.78,0.62,0.22"
	case strings.Contains(lower, "marginal") || strings.Contains(lower, "mrgl"):
		return "0.72,0.68,0.28"
	default:
		return "0.42,0.58,0.36"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
