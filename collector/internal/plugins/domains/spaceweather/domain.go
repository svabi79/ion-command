package spaceweather

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct{}
type rawState struct {
	SampleID          string  `json:"sampleId"`
	Kp                float64 `json:"kp"`
	AIndex            int     `json:"aIndex"`
	SolarFlux         int     `json:"solarFlux"`
	SolarWindSpeedKms int     `json:"solarWindSpeedKms"`
	SolarWindDensity  float64 `json:"solarWindDensity"`
	IMFBzNt           float64 `json:"imfBzNt"`
}

func New() *Domain               { return &Domain{} }
func (d *Domain) ID() string     { return "domain.spaceweather" }
func (d *Domain) Domain() string { return "spaceweather" }
func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawState
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode space-weather record: %w", err)
	}
	if raw.SampleID == "" {
		return nil, fmt.Errorf("space-weather sample requires id")
	}
	event := events.NewEnvelope(record.OriginalID, "spaceweather", "spaceweather.state", events.MessageObservation, events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}, record.ObservedUTC)
	event.Properties = map[string]any{"kp": raw.Kp, "aIndex": raw.AIndex, "solarFlux": raw.SolarFlux, "solarWindSpeedKms": raw.SolarWindSpeedKms, "solarWindDensity": raw.SolarWindDensity, "imfBzNt": raw.IMFBzNt}
	modelled := false
	event.Quality.Measured = &modelled
	return []events.Envelope{event}, nil
}
