// Package geography normalizes static cartography: named regions, country
// borders, cities, rivers, landmarks, and submarine-cable routes.
package geography

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
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
	case "border":
		return d.border(record)
	case "city":
		return d.city(record)
	case "river":
		return d.river(record)
	case "country":
		return d.country(record)
	case "peak", "landmark":
		return d.landmark(record)
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
		LOD        int     `json:"lod"`
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
		"visual.markerScale": 0.55,
		"visual.lod":         raw.LOD,
		"display.title":      raw.Name,
		"display.primary":    raw.RegionKind,
	}
	return []events.Envelope{event}, nil
}

func cartographicUntil(observed time.Time) time.Time {
	return observed.Add(30 * 24 * time.Hour)
}

func (d *Domain) border(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		BorderID string        `json:"borderId"`
		Name     string        `json:"name"`
		Segments [][][]float64 `json:"segments"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.BorderID == "" || len(raw.Segments) == 0 {
		return nil, fmt.Errorf("geography border requires id and line")
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.border", events.MessageRelationship,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:border:" + raw.BorderID
	if len(raw.Segments) == 1 {
		event.Geometry = events.LineString(raw.Segments[0])
	} else {
		event.Geometry = events.MultiLineString(raw.Segments)
	}
	validUntil := cartographicUntil(record.ObservedUTC)
	event.Time.ValidUntilUTC = &validUntil
	title := raw.Name
	if title == "" {
		title = "Country border"
	}
	event.Properties = map[string]any{
		"visual.color":       "0.40,0.46,0.52",
		"visual.legendIndex": 0,
		"display.title":      title,
		"display.primary":    "admin-0 boundary",
	}
	return []events.Envelope{event}, nil
}

func (d *Domain) city(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		PlaceID    string  `json:"placeId"`
		Name       string  `json:"name"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		Population int     `json:"population"`
		Capital    bool    `json:"capital"`
		LOD        int     `json:"lod"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.Name == "" {
		return nil, fmt.Errorf("geography city requires a name")
	}
	id := raw.PlaceID
	if id == "" {
		id = raw.Name
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.city", events.MessageAnnotation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:city:" + id
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := cartographicUntil(record.ObservedUTC)
	event.Time.ValidUntilUTC = &validUntil
	primary := "city"
	if raw.Capital {
		primary = "capital"
	}
	if raw.Population > 0 {
		primary = fmt.Sprintf("%s  //  %s", primary, formatPopulation(raw.Population))
	}
	event.Properties = map[string]any{
		"visual.icon":        "station",
		"visual.markerScale": 0.45,
		"visual.lod":         raw.LOD,
		"display.title":      raw.Name,
		"display.primary":    primary,
	}
	return []events.Envelope{event}, nil
}

func formatPopulation(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func (d *Domain) river(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		RiverID  string        `json:"riverId"`
		Name     string        `json:"name"`
		Segments [][][]float64 `json:"segments"`
		LabelLon float64       `json:"labelLon"`
		LabelLat float64       `json:"labelLat"`
		LOD      int           `json:"lod"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.RiverID == "" || raw.Name == "" || len(raw.Segments) == 0 {
		return nil, fmt.Errorf("geography river requires id, name and centerline")
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.river", events.MessageRelationship,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:river:" + raw.RiverID
	if len(raw.Segments) == 1 {
		event.Geometry = events.LineString(raw.Segments[0])
	} else {
		event.Geometry = events.MultiLineString(raw.Segments)
	}
	validUntil := cartographicUntil(record.ObservedUTC)
	event.Time.ValidUntilUTC = &validUntil
	event.Properties = map[string]any{
		"visual.color":       "0.26,0.40,0.50",
		"visual.legendIndex": 1,
		"visual.lod":         raw.LOD,
		"display.title":      raw.Name,
		"display.primary":    "river",
	}
	out := []events.Envelope{event}
	if raw.LabelLon != 0 || raw.LabelLat != 0 {
		label := events.NewEnvelope(record.OriginalID+":label", "geography", "geography.landmark", events.MessageAnnotation,
			events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID + ":label"},
			record.ObservedUTC)
		label.EntityID = "geography:riverlabel:" + raw.RiverID
		label.Geometry = events.Point(raw.LabelLon, raw.LabelLat, 0)
		label.Time.ValidUntilUTC = &validUntil
		label.Properties = map[string]any{
			"visual.icon":        "signal",
			"visual.markerScale": 0.4,
			"visual.lod":         raw.LOD,
			"display.title":      raw.Name,
			"display.primary":    "river",
		}
		out = append(out, label)
	}
	return out, nil
}

func (d *Domain) country(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		PlaceID   string  `json:"placeId"`
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		LOD       int     `json:"lod"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.Name == "" {
		return nil, fmt.Errorf("geography country requires a name")
	}
	id := raw.PlaceID
	if id == "" {
		id = raw.Name
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.country", events.MessageAnnotation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:country:" + id
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := cartographicUntil(record.ObservedUTC)
	event.Time.ValidUntilUTC = &validUntil
	event.Properties = map[string]any{
		"visual.icon":        "station",
		"visual.markerScale": 0.5,
		"visual.lod":         raw.LOD,
		"display.title":      raw.Name,
		"display.primary":    "country",
	}
	return []events.Envelope{event}, nil
}

func (d *Domain) landmark(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		PlaceID    string  `json:"placeId"`
		Name       string  `json:"name"`
		Kind       string  `json:"kind"`
		Latitude   float64 `json:"latitude"`
		Longitude  float64 `json:"longitude"`
		Elevation  int     `json:"elevation"`
		LOD        int     `json:"lod"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.Name == "" {
		return nil, fmt.Errorf("geography landmark requires a name")
	}
	id := raw.PlaceID
	if id == "" {
		id = raw.Name
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.landmark", events.MessageAnnotation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:landmark:" + id
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := cartographicUntil(record.ObservedUTC)
	event.Time.ValidUntilUTC = &validUntil
	primary := raw.Kind
	if primary == "" {
		primary = "landmark"
	}
	if raw.Elevation != 0 {
		primary = fmt.Sprintf("%s  //  %d m", primary, raw.Elevation)
	}
	event.Properties = map[string]any{
		"visual.icon":        "station",
		"visual.markerScale": 0.45,
		"visual.lod":         raw.LOD,
		"display.title":      raw.Name,
		"display.primary":    primary,
	}
	return []events.Envelope{event}, nil
}

func (d *Domain) cable(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		CableID  string        `json:"cableId"`
		Name     string        `json:"name"`
		Color    string        `json:"color"`
		Segments [][][]float64 `json:"segments"`
		Landings []string      `json:"landings"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.CableID == "" || len(raw.Segments) == 0 {
		return nil, fmt.Errorf("geography cable requires id and route")
	}
	event := events.NewEnvelope(record.OriginalID, "geography", "geography.cable", events.MessageRelationship,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "geography:cable:" + raw.CableID
	if len(raw.Segments) == 1 {
		event.Geometry = events.LineString(raw.Segments[0])
	} else {
		event.Geometry = events.MultiLineString(raw.Segments)
	}
	validUntil := record.ObservedUTC.Add(30 * 24 * time.Hour)
	event.Time.ValidUntilUTC = &validUntil
	lengthKm := routeLengthKm(raw.Segments)
	planned := isPlannedCableColor(raw.Color)
	class, color := cableLegend(planned, lengthKm)
	primary := class
	if lengthKm > 0 {
		primary = fmt.Sprintf("%s  //  %.0f km", class, lengthKm)
	}
	event.Properties = map[string]any{
		"visual.color":       color,
		"visual.legendIndex": cableLegendIndex(planned, lengthKm),
		"display.title":      raw.Name,
		"display.primary":    primary,
		"display.secondary":  formatLandingSecondary(raw.Landings),
	}
	return []events.Envelope{event}, nil
}

const maxNamedLandings = 8

func formatLandingSecondary(names []string) string {
	if len(names) == 0 {
		return "submarine cable"
	}
	if len(names) <= maxNamedLandings {
		return strings.Join(names, "  //  ")
	}
	return fmt.Sprintf("%s  //  +%d", strings.Join(names[:maxNamedLandings], "  //  "), len(names)-maxNamedLandings)
}

// TeleGeography paints planned / unbuilt cables #939597 on the public map.
// The geojson itself has no status field; this is the feed's operational cue.
func isPlannedCableColor(color string) bool {
	return strings.EqualFold(strings.TrimSpace(color), "#939597")
}

func routeLengthKm(segments [][][]float64) float64 {
	const earthKm = 6371.0
	total := 0.0
	for _, line := range segments {
		for i := 1; i < len(line); i++ {
			if len(line[i-1]) < 2 || len(line[i]) < 2 {
				continue
			}
			lat1 := line[i-1][1] * math.Pi / 180
			lat2 := line[i][1] * math.Pi / 180
			dLat := lat2 - lat1
			dLon := (line[i][0] - line[i-1][0]) * math.Pi / 180
			sinLat := math.Sin(dLat / 2)
			sinLon := math.Sin(dLon / 2)
			h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
			total += 2 * earthKm * math.Asin(math.Min(1, math.Sqrt(h)))
		}
	}
	return total
}

// Globe-side linear RGB. Must match AGeoPathLayerActor LegendColors.
// Same five classes; muted so additive MI_Track reads as map lines.
func cableLegend(planned bool, lengthKm float64) (class, color string) {
	if planned {
		return "planned", "0.78,0.58,0.26"
	}
	switch {
	case lengthKm < 500:
		return "in service · short", "0.30,0.56,0.50"
	case lengthKm < 3000:
		return "in service · regional", "0.34,0.60,0.70"
	case lengthKm < 12000:
		return "in service · ocean", "0.30,0.42,0.66"
	default:
		return "in service · trunk", "0.66,0.34,0.52"
	}
}

func cableLegendIndex(planned bool, lengthKm float64) int {
	if planned {
		return 0
	}
	switch {
	case lengthKm < 500:
		return 1
	case lengthKm < 3000:
		return 2
	case lengthKm < 12000:
		return 3
	default:
		return 4
	}
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
