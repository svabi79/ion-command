// Extract helpers turn Natural Earth 110m GeoJSON into the compact
// JSON files embedded next to this package. The cook lives in
// collector/cmd/neextract; bumping to 50m later is the same pipeline
// against a different input directory.
package naturalearth

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	extractMaxVertices = 48
	extractMinVertexKm = 40.0
	earthKm            = 6371.0
)

type borderFeature struct {
	ID       string        `json:"id"`
	Name     string        `json:"name,omitempty"`
	Segments [][][]float64 `json:"segments"`
}

type placeFeature struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	Lon        float64 `json:"lon"`
	Lat        float64 `json:"lat"`
	LOD        int     `json:"lod"`
	Population int     `json:"population,omitempty"`
	Capital    bool    `json:"capital,omitempty"`
	Elevation  int     `json:"elevation,omitempty"`
}

type riverFeature struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	LOD      int           `json:"lod"`
	LabelLon float64       `json:"labelLon"`
	LabelLat float64       `json:"labelLat"`
	Segments [][][]float64 `json:"segments"`
}

type geoJSONFile struct {
	Features []geoJSONFeature `json:"features"`
}

type geoJSONFeature struct {
	Properties map[string]any `json:"properties"`
	Geometry   struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
}

func propertyString(properties map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := properties[key]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" && !strings.EqualFold(typed, "null") {
					return strings.TrimSpace(typed)
				}
			case float64:
				if typed != 0 {
					return strconv.FormatInt(int64(typed), 10)
				}
			}
		}
	}
	return ""
}

func propertyFloat(properties map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := properties[key]; ok {
			switch typed := value.(type) {
			case float64:
				return typed
			case json.Number:
				n, _ := typed.Float64()
				return n
			case string:
				n, _ := strconv.ParseFloat(typed, 64)
				return n
			}
		}
	}
	return 0
}

func parseLines(coords json.RawMessage) [][][]float64 {
	var line [][]float64
	if json.Unmarshal(coords, &line) == nil && validLine(line) {
		return splitDateline(line)
	}
	var multi [][][]float64
	if json.Unmarshal(coords, &multi) != nil {
		return nil
	}
	out := make([][][]float64, 0, len(multi))
	for _, segment := range multi {
		if validLine(segment) {
			out = append(out, splitDateline(segment)...)
		}
	}
	return out
}

func parsePoint(coords json.RawMessage) (lon, lat float64, ok bool) {
	var point []float64
	if json.Unmarshal(coords, &point) == nil && len(point) >= 2 {
		return point[0], point[1], point[0] >= -180 && point[0] <= 180 && point[1] >= -90 && point[1] <= 90
	}
	return 0, 0, false
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

func splitDateline(line [][]float64) [][][]float64 {
	if len(line) < 2 {
		return nil
	}
	parts := [][][]float64{}
	current := [][]float64{line[0]}
	for i := 1; i < len(line); i++ {
		prev := current[len(current)-1]
		if math.Abs(line[i][0]-prev[0]) > 180 {
			if len(current) >= 2 {
				parts = append(parts, current)
			}
			current = [][]float64{line[i]}
			continue
		}
		current = append(current, line[i])
	}
	if len(current) >= 2 {
		parts = append(parts, current)
	}
	if len(parts) == 0 {
		return nil
	}
	return parts
}

func distanceKm(a, b []float64) float64 {
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
		if distanceKm(last, line[i]) >= extractMinVertexKm {
			kept = append(kept, line[i])
			last = line[i]
		}
	}
	kept = append(kept, line[len(line)-1])
	if len(kept) <= extractMaxVertices {
		return kept
	}
	step := float64(len(kept)-1) / float64(extractMaxVertices-1)
	reduced := make([][]float64, 0, extractMaxVertices)
	for i := 0; i < extractMaxVertices-1; i++ {
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

func lineMidpoint(segments [][][]float64) (lon, lat float64) {
	best := [][]float64{}
	bestLen := 0.0
	for _, line := range segments {
		length := 0.0
		for i := 1; i < len(line); i++ {
			length += distanceKm(line[i-1], line[i])
		}
		if length > bestLen {
			bestLen = length
			best = line
		}
	}
	if len(best) == 0 {
		return 0, 0
	}
	target := bestLen / 2
	walked := 0.0
	for i := 1; i < len(best); i++ {
		step := distanceKm(best[i-1], best[i])
		if walked+step >= target || i == len(best)-1 {
			return best[i][0], best[i][1]
		}
		walked += step
	}
	return best[0][0], best[0][1]
}

func loadGeoJSON(path string) ([]geoJSONFeature, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file geoJSONFile
	if err := json.Unmarshal(body, &file); err != nil {
		return nil, fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return file.Features, nil
}

func cityLOD(population int, scalerank int, capital bool) int {
	switch {
	case population >= 5000000 || scalerank <= 1:
		return 0
	case population >= 1000000 || scalerank <= 3 || (capital && population >= 200000):
		return 1
	default:
		return 2
	}
}

func countryLOD(labelRank int) int {
	switch {
	case labelRank <= 2:
		return 0
	case labelRank <= 4:
		return 1
	default:
		return 2
	}
}

func peakLOD(elevation int, name string) int {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "everest") || elevation >= 8000 {
		return 0
	}
	if elevation >= 5000 || strings.Contains(lower, "denali") || strings.Contains(lower, "aconcagua") || strings.Contains(lower, "kilimanjaro") {
		return 1
	}
	return 2
}

func regionLOD(name, kind string) int {
	if kind == "marine" {
		switch name {
		case "Atlantic Ocean", "Pacific Ocean", "Indian Ocean", "Arctic Ocean", "Southern Ocean":
			return 0
		default:
			return 1
		}
	}
	switch name {
	case "Africa", "Antarctica", "Asia", "Australia", "Europe", "North America", "South America":
		return 0
	default:
		return 1
	}
}

func riverLOD(scalerank int, name string) int {
	if scalerank <= 1 || strings.EqualFold(name, "Amazon") || strings.EqualFold(name, "Nile") || strings.EqualFold(name, "Mississippi") {
		return 0
	}
	return 1
}

func cookBorders(features []geoJSONFeature) []borderFeature {
	out := make([]borderFeature, 0, len(features))
	for i, feature := range features {
		class := strings.ToLower(propertyString(feature.Properties, "FEATURECLA", "featurecla"))
		if strings.Contains(class, "disputed") || strings.Contains(class, "line of control") || strings.Contains(class, "claim") {
			continue
		}
		lines := decimateLines(parseLines(feature.Geometry.Coordinates))
		if len(lines) == 0 {
			continue
		}
		id := propertyString(feature.Properties, "NE_ID", "ne_id")
		if id == "" {
			id = fmt.Sprintf("border-%d", i)
		}
		out = append(out, borderFeature{
			ID:       id,
			Name:     propertyString(feature.Properties, "NAME", "name"),
			Segments: lines,
		})
	}
	return out
}

func cookPlaces(cities, peaks, countries []geoJSONFeature) []placeFeature {
	out := make([]placeFeature, 0, len(cities)+len(peaks)+len(countries))
	seen := map[string]struct{}{}
	add := func(place placeFeature) {
		if place.Name == "" {
			return
		}
		if _, ok := seen[place.Kind+":"+strings.ToLower(place.Name)]; ok {
			return
		}
		seen[place.Kind+":"+strings.ToLower(place.Name)] = struct{}{}
		out = append(out, place)
	}
	for i, feature := range cities {
		lon, lat, ok := parsePoint(feature.Geometry.Coordinates)
		if !ok {
			continue
		}
		name := propertyString(feature.Properties, "NAME", "name", "NAME_EN", "name_en")
		class := strings.ToLower(propertyString(feature.Properties, "FEATURECLA", "featurecla"))
		population := int(propertyFloat(feature.Properties, "POP_MAX", "pop_max"))
		scalerank := int(propertyFloat(feature.Properties, "SCALERANK", "scalerank"))
		capital := strings.Contains(class, "admin-0 capital")
		id := propertyString(feature.Properties, "NE_ID", "ne_id")
		if id == "" {
			id = fmt.Sprintf("city-%d", i)
		}
		add(placeFeature{
			ID: "city-" + id, Name: name, Kind: "city", Lon: lon, Lat: lat,
			LOD: cityLOD(population, scalerank, capital), Population: population, Capital: capital,
		})
	}
	for i, feature := range peaks {
		lon, lat, ok := parsePoint(feature.Geometry.Coordinates)
		if !ok {
			continue
		}
		name := propertyString(feature.Properties, "name_en", "name", "NAME_EN", "NAME")
		if name == "" {
			continue
		}
		elevation := int(propertyFloat(feature.Properties, "elevation"))
		id := propertyString(feature.Properties, "ne_id", "NE_ID")
		if id == "" {
			id = fmt.Sprintf("peak-%d", i)
		}
		add(placeFeature{
			ID: "peak-" + id, Name: name, Kind: "peak", Lon: lon, Lat: lat,
			LOD: peakLOD(elevation, name), Elevation: elevation,
		})
	}
	for i, feature := range countries {
		name := propertyString(feature.Properties, "NAME", "name")
		if name == "" {
			continue
		}
		lon := propertyFloat(feature.Properties, "LABEL_X", "label_x")
		lat := propertyFloat(feature.Properties, "LABEL_Y", "label_y")
		if lon < -180 || lon > 180 || lat < -90 || lat > 90 || (lon == 0 && lat == 0) {
			continue
		}
		labelRank := int(propertyFloat(feature.Properties, "LABELRANK", "labelrank"))
		id := propertyString(feature.Properties, "ADM0_A3", "adm0_a3")
		if id == "" {
			id = fmt.Sprintf("country-%d", i)
		}
		add(placeFeature{
			ID: "country-" + id, Name: name, Kind: "country", Lon: lon, Lat: lat,
			LOD: countryLOD(labelRank),
		})
	}
	return out
}

func cookRivers(features []geoJSONFeature) []riverFeature {
	out := make([]riverFeature, 0, len(features))
	seen := map[string]struct{}{}
	for i, feature := range features {
		class := strings.ToLower(propertyString(feature.Properties, "featurecla", "FEATURECLA"))
		if class != "" && !strings.Contains(class, "river") {
			continue
		}
		name := propertyString(feature.Properties, "name_en", "name", "label")
		if name == "" {
			continue
		}
		lines := decimateLines(parseLines(feature.Geometry.Coordinates))
		if len(lines) == 0 {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		id := propertyString(feature.Properties, "ne_id", "NE_ID")
		if id == "" {
			id = fmt.Sprintf("river-%d", i)
		}
		labelLon, labelLat := lineMidpoint(lines)
		out = append(out, riverFeature{
			ID: id, Name: name, LOD: riverLOD(int(propertyFloat(feature.Properties, "scalerank", "SCALERANK")), name),
			LabelLon: labelLon, LabelLat: labelLat, Segments: lines,
		})
	}
	return out
}

// CookExtracts writes borders.json, places.json and rivers.json into outDir
// from a directory of Natural Earth 110m GeoJSON files.
func CookExtracts(inputDir, outDir string) error {
	bordersIn, err := loadGeoJSON(filepath.Join(inputDir, "ne_110m_admin_0_boundary_lines_land.geojson"))
	if err != nil {
		return err
	}
	citiesIn, err := loadGeoJSON(filepath.Join(inputDir, "ne_110m_populated_places.geojson"))
	if err != nil {
		return err
	}
	peaksIn, err := loadGeoJSON(filepath.Join(inputDir, "ne_110m_geography_regions_elevation_points.geojson"))
	if err != nil {
		return err
	}
	countriesIn, err := loadGeoJSON(filepath.Join(inputDir, "ne_110m_admin_0_countries.geojson"))
	if err != nil {
		return err
	}
	riversIn, err := loadGeoJSON(filepath.Join(inputDir, "ne_110m_rivers_lake_centerlines.geojson"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	writes := []struct {
		name string
		body any
	}{
		{"borders.json", cookBorders(bordersIn)},
		{"places.json", cookPlaces(citiesIn, peaksIn, countriesIn)},
		{"rivers.json", cookRivers(riversIn)},
	}
	for _, write := range writes {
		encoded, err := json.Marshal(write.body)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, write.name), encoded, 0o644); err != nil {
			return err
		}
	}
	return nil
}
