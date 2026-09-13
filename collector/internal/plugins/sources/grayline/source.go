// Package grayline emits a derived solar-terminator / nautical-twilight
// band as raw polygon rings. No external API: the geometry is computed
// from UTC using the same subsolar model as the Unreal globe lighting.
package grayline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/solar"
)

const (
	pollDefault = 2 * time.Minute
	pollFloor   = 30 * time.Second
)

type Source struct {
	id       string
	interval time.Duration
	logger   *slog.Logger
	now      func() time.Time
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "solar.grayline" {
		return nil, fmt.Errorf("unsupported grayline source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		interval = pollFloor
	}
	return &Source{id: sourceConfig.ID, interval: interval, logger: logger, now: time.Now}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "solar.grayline" }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	s.emit(ctx, output)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.emit(ctx, output)
		}
	}
}

func (s *Source) emit(ctx context.Context, output chan<- plugins.RawRecord) {
	record, ok := RecordAt(s.now().UTC(), s.id)
	if !ok {
		s.logger.Warn("grayline band produced no ring", "source", s.id)
		return
	}
	select {
	case output <- record:
	case <-ctx.Done():
	}
}

// RecordAt builds the raw grayline record for a UTC instant.
func RecordAt(at time.Time, sourceInstanceID string) (plugins.RawRecord, bool) {
	at = at.UTC()
	lat, lon := solar.Subsolar(at.Unix(), at.Nanosecond())
	ring := solar.TwilightBand(lat, lon)
	if len(ring) < 8 {
		return plugins.RawRecord{}, false
	}
	payload, err := json.Marshal(map[string]any{
		"kind":              "grayline",
		"rings":             [][][]float64{ring},
		"subsolarLatitude":  lat,
		"subsolarLongitude": lon,
		"bandInnerDeg":      90.0,
		"bandOuterDeg":      102.0,
	})
	if err != nil {
		return plugins.RawRecord{}, false
	}
	return plugins.RawRecord{
		SourcePluginID:   "grayline",
		SourceInstanceID: sourceInstanceID,
		OriginalID:       fmt.Sprintf("grayline-%d", at.Unix()),
		Domain:           "solar",
		ObservedUTC:      at,
		Payload:          payload,
	}, true
}
