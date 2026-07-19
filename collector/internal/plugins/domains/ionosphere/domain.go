// Package ionosphere normalizes ionosonde soundings (foF2, hmF2, foE,
// MUF(3000)F2, TEC) into canonical per-station observations.
package ionosphere

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct{}

type rawSounding struct {
	StationID  string   `json:"stationId"`
	Name       string   `json:"name"`
	Latitude   float64  `json:"latitude"`
	Longitude  float64  `json:"longitude"`
	FoF2Mhz    float64  `json:"foF2Mhz"`
	MufDMhz    float64  `json:"mufdMhz"`
	HmF2Km     *float64 `json:"hmF2Km"`
	FoEMhz     *float64 `json:"foEMhz"`
	M3000      *float64 `json:"m3000"`
	TEC        *float64 `json:"tec"`
	Confidence *float64 `json:"confidence"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.ionosphere" }
func (d *Domain) Domain() string { return "ionosphere" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawSounding
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode ionosonde record: %w", err)
	}
	if raw.StationID == "" || raw.FoF2Mhz <= 0 || raw.MufDMhz <= 0 {
		return nil, fmt.Errorf("sounding requires station id, foF2, and MUF")
	}
	event := events.NewEnvelope(record.OriginalID, "ionosphere", "ionosphere.sounding", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	event.EntityID = "ionosphere:station:" + raw.StationID
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	event.Properties = map[string]any{
		"stationId":     raw.StationID,
		"foF2Mhz":       raw.FoF2Mhz,
		"mufdMhz":       raw.MufDMhz,
		"visual.icon":   "sounding",
		"display.title": fmt.Sprintf("Ionosonde %s", raw.StationID),
		"display.primary": fmt.Sprintf("foF2 %.1f MHz  //  MUF(3000) %.1f MHz",
			raw.FoF2Mhz, raw.MufDMhz),
	}
	if raw.Name != "" {
		event.Properties["display.secondary"] = raw.Name
	}
	if raw.HmF2Km != nil {
		event.Properties["hmF2Km"] = *raw.HmF2Km
	}
	if raw.FoEMhz != nil {
		event.Properties["foEMhz"] = *raw.FoEMhz
	}
	if raw.M3000 != nil {
		event.Properties["m3000"] = *raw.M3000
	}
	if raw.TEC != nil {
		event.Properties["tec"] = *raw.TEC
	}
	if raw.Confidence != nil {
		event.Properties["confidence"] = *raw.Confidence
	}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}
