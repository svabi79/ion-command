// Package solar normalizes derived solar-geometry records (grayline /
// terminator band) into Area envelopes. No ham-radio vocabulary.
package solar

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct{}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.solar" }
func (d *Domain) Domain() string { return "solar" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw struct {
		Kind      string        `json:"kind"`
		Rings     [][][]float64 `json:"rings"`
		SubLat    float64       `json:"subsolarLatitude"`
		SubLon    float64       `json:"subsolarLongitude"`
		BandInner float64       `json:"bandInnerDeg"`
		BandOuter float64       `json:"bandOuterDeg"`
	}
	if err := json.Unmarshal(record.Payload, &raw); err != nil || raw.Kind != "grayline" || len(raw.Rings) == 0 {
		return nil, fmt.Errorf("decode solar grayline")
	}
	event := events.NewEnvelope(record.OriginalID, "solar", "solar.grayline", events.MessageArea,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "solar:grayline"
	event.Geometry = events.Polygon(raw.Rings)
	validUntil := record.ObservedUTC.Add(5 * time.Minute)
	event.Time.ValidUntilUTC = &validUntil
	event.Properties = map[string]any{
		"visual.color":      "0.52,0.48,0.38",
		"visual.opacity":    0.12,
		"display.title":     "Grayline",
		"display.primary":   "solar terminator / nautical twilight",
		"display.secondary": fmt.Sprintf("subsolar %.1f, %.1f", raw.SubLat, raw.SubLon),
	}
	if raw.BandInner > 0 {
		event.Properties["bandInnerDeg"] = raw.BandInner
	}
	if raw.BandOuter > 0 {
		event.Properties["bandOuterDeg"] = raw.BandOuter
	}
	modeled := false
	event.Quality.Measured = &modeled
	event.Quality.Classification = "modelled"
	return []events.Envelope{event}, nil
}
