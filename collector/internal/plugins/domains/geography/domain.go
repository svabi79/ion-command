// Package geography normalizes static cartographic labels: named regions
// and submarine-cable routes.
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
		"visual.markerScale": 0.7,
		"display.title":      raw.Name,
		"display.primary":    raw.RegionKind,
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
