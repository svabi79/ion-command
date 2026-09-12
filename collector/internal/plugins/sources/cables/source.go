// Package cables fetches TeleGeography's public submarine-cable map
// (cables plus landing points), simplifies each cable to a great-circle
// between its first and last vertices, and caches the copy in a removable
// folder. Licensed CC BY-NC-SA 3.0; attribution is required.
package cables

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultCablesURL   = "https://www.submarinecablemap.com/api/v3/cable/cable-geo.json"
	defaultLandingsURL = "https://www.submarinecablemap.com/api/v3/landing-point/landing-point-geo.json"
	pollDefault        = 24 * time.Hour
	pollFloor          = 6 * time.Hour
	maxCables          = 250
	maxLandings        = 400
	attribution        = "Submarine cables © TeleGeography (CC BY-NC-SA 3.0)"
)

type Source struct {
	id           string
	cablesURL    string
	landingsURL  string
	interval     time.Duration
	cache        pollutil.FileCache
	client       *http.Client
	logger       *slog.Logger
	fetchCables  func(ctx context.Context) ([]byte, error)
	fetchLanding func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "geography.cables" {
		return nil, fmt.Errorf("unsupported cables source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("cable poll interval below six hours (got %s)", interval)
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "cables"))
	source := &Source{
		id:          sourceConfig.ID,
		cablesURL:   defaultCablesURL,
		landingsURL: defaultLandingsURL,
		interval:    interval,
		cache:       pollutil.FileCache{Path: filepath.Join(cacheDir, "simplified.json"), TTL: interval},
		client:      &http.Client{Timeout: 90 * time.Second},
		logger:      logger,
	}
	if sourceConfig.Broker != "" {
		source.cablesURL = sourceConfig.Broker
	}
	if sourceConfig.Topic != "" {
		source.landingsURL = sourceConfig.Topic
	}
	source.fetchCables = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.cablesURL, nil)
	}
	source.fetchLanding = func(ctx context.Context) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.landingsURL, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.cables" }

type geoJSON struct {
	Features []struct {
		Properties map[string]any `json:"properties"`
		Geometry   struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

func propertyString(properties map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := properties[key]; ok {
			if text, ok := value.(string); ok && text != "" {
				return text
			}
		}
	}
	return ""
}

func firstLast(coords json.RawMessage) (lon1, lat1, lon2, lat2 float64, ok bool) {
	var line [][]float64
	if json.Unmarshal(coords, &line) == nil && len(line) >= 2 && len(line[0]) >= 2 && len(line[len(line)-1]) >= 2 {
		return line[0][0], line[0][1], line[len(line)-1][0], line[len(line)-1][1], true
	}
	var multi [][][]float64
	if json.Unmarshal(coords, &multi) == nil && len(multi) > 0 && len(multi[0]) >= 2 {
		first, last := multi[0][0], multi[len(multi)-1][len(multi[len(multi)-1])-1]
		if len(first) >= 2 && len(last) >= 2 {
			return first[0], first[1], last[0], last[1], true
		}
	}
	return 0, 0, 0, 0, false
}

func pointCoord(coords json.RawMessage) (lon, lat float64, ok bool) {
	var point []float64
	if json.Unmarshal(coords, &point) == nil && len(point) >= 2 {
		return point[0], point[1], true
	}
	return 0, 0, false
}

type simplified struct {
	Cables   []simplifiedCable  `json:"cables"`
	Landings []simplifiedLanding `json:"landings"`
}

type simplifiedCable struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lon1 float64 `json:"lon1"`
	Lat1 float64 `json:"lat1"`
	Lon2 float64 `json:"lon2"`
	Lat2 float64 `json:"lat2"`
}

type simplifiedLanding struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

func (s *Source) refresh(ctx context.Context) (simplified, error) {
	if cached, _, ok := s.cache.Load(); ok {
		var ready simplified
		if json.Unmarshal(cached, &ready) == nil {
			return ready, nil
		}
	}
	cablesBody, err := s.fetchCables(ctx)
	if err != nil {
		if cached, _, ok := s.cache.Load(); ok {
			var ready simplified
			if json.Unmarshal(cached, &ready) == nil {
				return ready, nil
			}
		}
		return simplified{}, err
	}
	landingsBody, err := s.fetchLanding(ctx)
	if err != nil {
		landingsBody = []byte(`{"features":[]}`)
	}
	var cables geoJSON
	if err := json.Unmarshal(cablesBody, &cables); err != nil {
		return simplified{}, fmt.Errorf("decode cable geojson: %w", err)
	}
	var landings geoJSON
	_ = json.Unmarshal(landingsBody, &landings)
	out := simplified{}
	for _, feature := range cables.Features {
		lon1, lat1, lon2, lat2, ok := firstLast(feature.Geometry.Coordinates)
		if !ok {
			continue
		}
		name := propertyString(feature.Properties, "name", "color", "id")
		id := propertyString(feature.Properties, "id", "name")
		if id == "" {
			id = fmt.Sprintf("%s-%d", name, len(out.Cables))
		}
		out.Cables = append(out.Cables, simplifiedCable{ID: id, Name: name, Lon1: lon1, Lat1: lat1, Lon2: lon2, Lat2: lat2})
		if len(out.Cables) >= maxCables {
			break
		}
	}
	for _, feature := range landings.Features {
		lon, lat, ok := pointCoord(feature.Geometry.Coordinates)
		if !ok {
			continue
		}
		name := propertyString(feature.Properties, "name", "id")
		id := propertyString(feature.Properties, "id", "name")
		if id == "" {
			id = fmt.Sprintf("lp-%d", len(out.Landings))
		}
		out.Landings = append(out.Landings, simplifiedLanding{ID: id, Name: name, Lon: lon, Lat: lat})
		if len(out.Landings) >= maxLandings {
			break
		}
	}
	if encoded, err := json.Marshal(out); err == nil {
		_ = s.cache.Store(encoded)
		_ = os.WriteFile(filepath.Join(filepath.Dir(s.cache.Path), "ATTRIBUTION.txt"), []byte(attribution+"\nhttps://www.submarinecablemap.com/\n"), 0o644)
	}
	return out, nil
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	data, err := s.refresh(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(data.Cables)+len(data.Landings))
	for _, cable := range data.Cables {
		payload, err := json.Marshal(map[string]any{
			"kind":        "cable",
			"cableId":     cable.ID,
			"name":        cable.Name,
			"fromLon":     cable.Lon1,
			"fromLat":     cable.Lat1,
			"toLon":       cable.Lon2,
			"toLat":       cable.Lat2,
			"attribution": attribution,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "cables", SourceInstanceID: s.id, OriginalID: "cable-" + cable.ID,
			Domain: "geography", ObservedUTC: now, Payload: payload,
		})
	}
	for _, landing := range data.Landings {
		payload, err := json.Marshal(map[string]any{
			"kind":        "landing",
			"landingId":   landing.ID,
			"name":        landing.Name,
			"longitude":   landing.Lon,
			"latitude":    landing.Lat,
			"attribution": attribution,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "cables", SourceInstanceID: s.id, OriginalID: "landing-" + landing.ID,
			Domain: "geography", ObservedUTC: now, Payload: payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("cables sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("cables snapshot", "source", s.id, "records", len(records))
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.interval):
		}
	}
}
