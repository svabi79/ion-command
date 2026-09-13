// Package cables fetches TeleGeography's public submarine-cable map
// (cables plus landing points), keeps each cable's MultiLineString
// route after a bounded decimate, and caches the copy in a removable
// folder. Licensed CC BY-NC-SA 3.0; attribution is required.
package cables

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	maxCables          = 800
	maxLandings        = 400
	maxLandingJoin     = 2500
	maxLinkedLandings  = 24
	maxVerticesPerLine = 64
	minVertexKm        = 25.0
	// Public cable-geo.json has no landing ids. A landing is related when it
	// sits near a kept segment endpoint (decimate preserves ends).
	landingLinkKm = 80.0
	attribution   = "Submarine cables © TeleGeography (CC BY-NC-SA 3.0)"
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
		cache:       pollutil.FileCache{Path: filepath.Join(cacheDir, "routes.json"), TTL: interval},
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

func parseLines(coords json.RawMessage) [][][]float64 {
	var line [][]float64
	if json.Unmarshal(coords, &line) == nil && validLine(line) {
		return [][][]float64{line}
	}
	var multi [][][]float64
	if json.Unmarshal(coords, &multi) != nil {
		return nil
	}
	out := make([][][]float64, 0, len(multi))
	for _, segment := range multi {
		if validLine(segment) {
			out = append(out, segment)
		}
	}
	return out
}

func validLine(line [][]float64) bool {
	if len(line) < 2 {
		return false
	}
	for _, point := range line {
		if len(point) < 2 || point[0] < -180 || point[0] > 180 || point[1] < -90 || point[1] > 90 {
			return false
		}
	}
	return true
}

func distanceKm(a, b []float64) float64 {
	const earthKm = 6371.0
	lat1 := a[1] * math.Pi / 180
	lat2 := b[1] * math.Pi / 180
	dLat := lat2 - lat1
	dLon := (b[0] - a[0]) * math.Pi / 180
	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)
	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
	return 2 * earthKm * math.Asin(math.Min(1, math.Sqrt(h)))
}

func decimateLine(line [][]float64) [][]float64 {
	if len(line) <= 2 {
		return line
	}
	kept := make([][]float64, 0, len(line))
	kept = append(kept, line[0])
	last := line[0]
	for i := 1; i < len(line)-1; i++ {
		if distanceKm(last, line[i]) >= minVertexKm {
			kept = append(kept, line[i])
			last = line[i]
		}
	}
	kept = append(kept, line[len(line)-1])
	if len(kept) <= maxVerticesPerLine {
		return kept
	}
	step := float64(len(kept)-1) / float64(maxVerticesPerLine-1)
	reduced := make([][]float64, 0, maxVerticesPerLine)
	for i := 0; i < maxVerticesPerLine-1; i++ {
		reduced = append(reduced, kept[int(float64(i)*step)])
	}
	return append(reduced, kept[len(kept)-1])
}

func decimateLines(lines [][][]float64) [][][]float64 {
	out := make([][][]float64, 0, len(lines))
	for _, line := range lines {
		if reduced := decimateLine(line); validLine(reduced) {
			out = append(out, reduced)
		}
	}
	return out
}

func landingNearCable(landing simplifiedLanding, cable routedCable, maxKm float64) bool {
	point := []float64{landing.Lon, landing.Lat}
	for _, segment := range cable.Segments {
		if len(segment) < 2 {
			continue
		}
		if distanceKm(point, segment[0]) <= maxKm || distanceKm(point, segment[len(segment)-1]) <= maxKm {
			return true
		}
	}
	return false
}

func linkLandingNames(cables []routedCable, landings []simplifiedLanding) {
	for i := range cables {
		seen := make(map[string]struct{})
		names := make([]string, 0)
		for _, landing := range landings {
			if landing.Name == "" {
				continue
			}
			if _, ok := seen[landing.Name]; ok {
				continue
			}
			if !landingNearCable(landing, cables[i], landingLinkKm) {
				continue
			}
			seen[landing.Name] = struct{}{}
			names = append(names, landing.Name)
			if len(names) >= maxLinkedLandings {
				break
			}
		}
		cables[i].Landings = names
	}
}

func pointCoord(coords json.RawMessage) (lon, lat float64, ok bool) {
	var point []float64
	if json.Unmarshal(coords, &point) == nil && len(point) >= 2 {
		return point[0], point[1], true
	}
	return 0, 0, false
}

type routed struct {
	Cables   []routedCable       `json:"cables"`
	Landings []simplifiedLanding `json:"landings"`
}

type routedCable struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Color    string        `json:"color,omitempty"`
	Segments [][][]float64 `json:"segments"`
	Landings []string      `json:"landings,omitempty"`
}

type simplifiedLanding struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lon  float64 `json:"lon"`
	Lat  float64 `json:"lat"`
}

func (s *Source) refresh(ctx context.Context) (routed, error) {
	if cached, _, ok := s.cache.Load(); ok {
		var ready routed
		if json.Unmarshal(cached, &ready) == nil && len(ready.Cables) > 0 {
			return ready, nil
		}
	}
	cablesBody, err := s.fetchCables(ctx)
	if err != nil {
		if cached, _, ok := s.cache.Load(); ok {
			var ready routed
			if json.Unmarshal(cached, &ready) == nil && len(ready.Cables) > 0 {
				return ready, nil
			}
		}
		return routed{}, err
	}
	landingsBody, err := s.fetchLanding(ctx)
	if err != nil {
		landingsBody = []byte(`{"features":[]}`)
	}
	var cables geoJSON
	if err := json.Unmarshal(cablesBody, &cables); err != nil {
		return routed{}, fmt.Errorf("decode cable geojson: %w", err)
	}
	var landings geoJSON
	_ = json.Unmarshal(landingsBody, &landings)
	byID := make(map[string]*routedCable, len(cables.Features))
	order := make([]string, 0, len(cables.Features))
	dropped := 0
	for _, feature := range cables.Features {
		lines := decimateLines(parseLines(feature.Geometry.Coordinates))
		if len(lines) == 0 {
			dropped++
			continue
		}
		name := propertyString(feature.Properties, "name", "id")
		id := propertyString(feature.Properties, "id", "name")
		if id == "" {
			id = fmt.Sprintf("%s-%d", name, len(order))
		}
		if existing, ok := byID[id]; ok {
			existing.Segments = append(existing.Segments, lines...)
			if existing.Color == "" {
				existing.Color = propertyString(feature.Properties, "color")
			}
			continue
		}
		if len(order) >= maxCables {
			dropped++
			continue
		}
		cable := &routedCable{
			ID:       id,
			Name:     name,
			Color:    propertyString(feature.Properties, "color"),
			Segments: lines,
		}
		byID[id] = cable
		order = append(order, id)
	}
	out := routed{Cables: make([]routedCable, 0, len(order))}
	for _, id := range order {
		out.Cables = append(out.Cables, *byID[id])
	}
	allLandings := make([]simplifiedLanding, 0, len(landings.Features))
	for _, feature := range landings.Features {
		lon, lat, ok := pointCoord(feature.Geometry.Coordinates)
		if !ok {
			continue
		}
		name := propertyString(feature.Properties, "name", "id")
		id := propertyString(feature.Properties, "id", "name")
		if id == "" {
			id = fmt.Sprintf("lp-%d", len(allLandings))
		}
		allLandings = append(allLandings, simplifiedLanding{ID: id, Name: name, Lon: lon, Lat: lat})
		if len(allLandings) >= maxLandingJoin {
			break
		}
	}
	linkLandingNames(out.Cables, allLandings)
	if len(allLandings) > maxLandings {
		out.Landings = allLandings[:maxLandings]
	} else {
		out.Landings = allLandings
	}
	if dropped > 0 {
		s.logger.Info("cables dropped", "source", s.id, "dropped", dropped, "kept", len(out.Cables))
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
		payloadMap := map[string]any{
			"kind":        "cable",
			"cableId":     cable.ID,
			"name":        cable.Name,
			"color":       strings.ToLower(cable.Color),
			"segments":    cable.Segments,
			"attribution": attribution,
		}
		if len(cable.Landings) > 0 {
			payloadMap["landings"] = cable.Landings
		}
		payload, err := json.Marshal(payloadMap)
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
