// Package naturalearth emits named 10m geography regions and marine
// polygons as globe labels. The bundled coordinates are a simplified
// public-domain extract of Natural Earth (naturalearthdata.com).
package naturalearth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"

	_ "embed"
)

//go:embed data/regions.json
var bundledRegions []byte

type feature struct {
	Name string  `json:"name"`
	Kind string  `json:"kind"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

type Source struct {
	id      string
	logger  *slog.Logger
	load    func() ([]feature, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "geography.naturalearth" {
		return nil, fmt.Errorf("unsupported natural earth source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	source := &Source{id: sourceConfig.ID, logger: logger}
	source.load = func() ([]feature, error) {
		body := bundledRegions
		if sourceConfig.Broker != "" {
			loaded, err := os.ReadFile(sourceConfig.Broker)
			if err != nil {
				return nil, err
			}
			body = loaded
		}
		var features []feature
		if err := json.Unmarshal(body, &features); err != nil {
			return nil, fmt.Errorf("decode natural earth regions: %w", err)
		}
		return features, nil
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.naturalearth" }

func (s *Source) sample() ([]plugins.RawRecord, error) {
	features, err := s.load()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(features))
	for _, feature := range features {
		if feature.Name == "" {
			continue
		}
		kind := feature.Kind
		if kind == "" {
			kind = "region"
		}
		payload, err := json.Marshal(map[string]any{
			"kind":      "region",
			"name":      feature.Name,
			"regionKind": kind,
			"latitude":  feature.Lat,
			"longitude": feature.Lon,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID:   "naturalearth",
			SourceInstanceID: s.id,
			OriginalID:       "ne-" + feature.Name,
			Domain:           "geography",
			ObservedUTC:      now,
			Payload:          payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	records, err := s.sample()
	if err != nil {
		return err
	}
	s.logger.Info("natural earth regions", "source", s.id, "features", len(records))
	for _, record := range records {
		select {
		case output <- record:
		case <-ctx.Done():
			return nil
		}
	}
	<-ctx.Done()
	return nil
}
