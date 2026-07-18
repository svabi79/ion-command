package weather

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct{}

type rawLightning struct {
	StrikeID      string  `json:"strikeId"`
	Longitude     float64 `json:"longitude"`
	Latitude      float64 `json:"latitude"`
	PeakCurrentKa float64 `json:"peakCurrentKa"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.weather" }
func (d *Domain) Domain() string { return "weather" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawLightning
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode lightning record: %w", err)
	}
	if raw.StrikeID == "" {
		return nil, fmt.Errorf("lightning strike requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "weather", "weather.lightning", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	event.Properties = map[string]any{"peakCurrentKa": raw.PeakCurrentKa, "unit": "kA"}
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}, nil
}
