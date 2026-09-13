// Package naturalearth emits a bundled public-domain Natural Earth 110m
// extract: country borders, named regions/oceans, cities, rivers, and
// physical landmarks. Coordinates are WGS84. To raise detail later, rerun
// collector/cmd/neextract against 50m GeoJSON; keep the same JSON schema.
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

	"embed"
)

// Bundled next to this file. A path under data/ is swallowed by the
// repo-root data/ gitignore (runtime caches) and cannot be go:embed'd.
//
//go:embed regions.json borders.json places.json rivers.json
var bundled embed.FS

type regionFeature struct {
	Name string  `json:"name"`
	Kind string  `json:"kind"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

type Source struct {
	id     string
	logger *slog.Logger
	load   func() ([]plugins.RawRecord, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "geography.naturalearth" {
		return nil, fmt.Errorf("unsupported natural earth source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	source := &Source{id: sourceConfig.ID, logger: logger}
	source.load = func() ([]plugins.RawRecord, error) {
		return source.sampleFromBundle(sourceConfig.Broker)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.naturalearth" }

func readBundledOrFile(name, override string) ([]byte, error) {
	if override != "" {
		return os.ReadFile(override)
	}
	return bundled.ReadFile(name)
}

func (s *Source) sampleFromBundle(regionsOverride string) ([]plugins.RawRecord, error) {
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, 1024)

	regionBody, err := readBundledOrFile("regions.json", regionsOverride)
	if err != nil {
		return nil, fmt.Errorf("natural earth regions: %w", err)
	}
	var regions []regionFeature
	if err := json.Unmarshal(regionBody, &regions); err != nil {
		return nil, fmt.Errorf("decode natural earth regions: %w", err)
	}
	for _, feature := range regions {
		if feature.Name == "" {
			continue
		}
		kind := feature.Kind
		if kind == "" {
			kind = "region"
		}
		records = appendRecord(records, s.id, now, "ne-region-"+feature.Name, map[string]any{
			"kind":       "region",
			"name":       feature.Name,
			"regionKind": kind,
			"latitude":   feature.Lat,
			"longitude":  feature.Lon,
			"lod":        regionLOD(feature.Name, kind),
		})
	}

	borderBody, err := bundled.ReadFile("borders.json")
	if err != nil {
		return nil, fmt.Errorf("natural earth borders: %w", err)
	}
	var borders []borderFeature
	if err := json.Unmarshal(borderBody, &borders); err != nil {
		return nil, fmt.Errorf("decode natural earth borders: %w", err)
	}
	for _, feature := range borders {
		if len(feature.Segments) == 0 {
			continue
		}
		records = appendRecord(records, s.id, now, "ne-border-"+feature.ID, map[string]any{
			"kind":     "border",
			"borderId": feature.ID,
			"name":     feature.Name,
			"segments": feature.Segments,
		})
	}

	placeBody, err := bundled.ReadFile("places.json")
	if err != nil {
		return nil, fmt.Errorf("natural earth places: %w", err)
	}
	var places []placeFeature
	if err := json.Unmarshal(placeBody, &places); err != nil {
		return nil, fmt.Errorf("decode natural earth places: %w", err)
	}
	for _, feature := range places {
		if feature.Name == "" {
			continue
		}
		payload := map[string]any{
			"kind":      feature.Kind,
			"name":      feature.Name,
			"latitude":  feature.Lat,
			"longitude": feature.Lon,
			"lod":       feature.LOD,
		}
		if feature.Kind == "city" {
			payload["kind"] = "city"
			payload["placeId"] = feature.ID
			if feature.Population > 0 {
				payload["population"] = feature.Population
			}
			payload["capital"] = feature.Capital
		} else {
			payload["kind"] = feature.Kind
			payload["placeId"] = feature.ID
			if feature.Elevation != 0 {
				payload["elevation"] = feature.Elevation
			}
		}
		records = appendRecord(records, s.id, now, "ne-"+feature.ID, payload)
	}

	riverBody, err := bundled.ReadFile("rivers.json")
	if err != nil {
		return nil, fmt.Errorf("natural earth rivers: %w", err)
	}
	var rivers []riverFeature
	if err := json.Unmarshal(riverBody, &rivers); err != nil {
		return nil, fmt.Errorf("decode natural earth rivers: %w", err)
	}
	for _, feature := range rivers {
		if feature.Name == "" || len(feature.Segments) == 0 {
			continue
		}
		records = appendRecord(records, s.id, now, "ne-river-"+feature.ID, map[string]any{
			"kind":     "river",
			"riverId":  feature.ID,
			"name":     feature.Name,
			"segments": feature.Segments,
			"labelLon": feature.LabelLon,
			"labelLat": feature.LabelLat,
			"lod":      feature.LOD,
		})
	}
	return records, nil
}

func appendRecord(records []plugins.RawRecord, instanceID string, now time.Time, originalID string, payloadMap map[string]any) []plugins.RawRecord {
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return records
	}
	return append(records, plugins.RawRecord{
		SourcePluginID:   "naturalearth",
		SourceInstanceID: instanceID,
		OriginalID:       originalID,
		Domain:           "geography",
		ObservedUTC:      now,
		Payload:          payload,
	})
}

func (s *Source) sample() ([]plugins.RawRecord, error) {
	return s.load()
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	records, err := s.sample()
	if err != nil {
		return err
	}
	s.logger.Info("natural earth cartography", "source", s.id, "features", len(records))
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
