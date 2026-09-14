// Package geoutil holds generic GeoJSON parsing and globe-scale
// simplification. No provider or domain vocabulary.
package geoutil

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type FeatureCollection struct {
	Features []Feature `json:"features"`
}

type Feature struct {
	ID         any            `json:"id"`
	Properties map[string]any `json:"properties"`
	Geometry   Geometry       `json:"geometry"`
}

type Geometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

func ParseFeatureCollection(body []byte) (FeatureCollection, error) {
	var collection FeatureCollection
	if err := json.Unmarshal(body, &collection); err != nil {
		return FeatureCollection{}, fmt.Errorf("decode geojson: %w", err)
	}
	return collection, nil
}

func FeatureID(feature Feature, keys ...string) string {
	if text := asString(feature.ID); text != "" && text != "<nil>" {
		return text
	}
	return PropertyString(feature.Properties, keys...)
}

func PropertyString(properties map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := asString(properties[key]); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func PropertyFloat(properties map[string]any, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := asFloat(properties[key]); ok {
			return value
		}
	}
	return 0
}

func asString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == math.Trunc(typed) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func asFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		n, err := typed.Float64()
		return n, err == nil
	case int:
		return float64(typed), true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func Point(geom Geometry) (lon, lat float64, ok bool) {
	if geom.Type == "" || strings.EqualFold(geom.Type, "Point") {
		var point []float64
		if json.Unmarshal(geom.Coordinates, &point) == nil && validPosition(point) {
			return point[0], point[1], true
		}
	}
	return 0, 0, false
}

// Polygons returns each polygon as a slice of rings (exterior first).
// MultiPolygon, Polygon, and closed LineString/MultiLineString are accepted.
func Polygons(geom Geometry) [][][][]float64 {
	switch geom.Type {
	case "Polygon":
		var rings [][][]float64
		if json.Unmarshal(geom.Coordinates, &rings) == nil {
			if cleaned := cleanPolygon(rings); len(cleaned) > 0 {
				return [][][][]float64{cleaned}
			}
		}
	case "MultiPolygon":
		var polygons [][][][]float64
		if json.Unmarshal(geom.Coordinates, &polygons) == nil {
			out := make([][][][]float64, 0, len(polygons))
			for _, rings := range polygons {
				if cleaned := cleanPolygon(rings); len(cleaned) > 0 {
					out = append(out, cleaned)
				}
			}
			return out
		}
	case "LineString":
		var line [][]float64
		if json.Unmarshal(geom.Coordinates, &line) == nil && alreadyClosed(line) {
			return [][][][]float64{{line}}
		}
	case "MultiLineString":
		var lines [][][]float64
		if json.Unmarshal(geom.Coordinates, &lines) == nil {
			out := make([][][][]float64, 0)
			for _, line := range lines {
				if alreadyClosed(line) {
					out = append(out, [][][]float64{line})
				}
			}
			return out
		}
	}
	return nil
}

// Lines returns LineString / MultiLineString segments, and the outer rings
// of polygons as open-or-closed lines for cartographic outlines.
func Lines(geom Geometry) [][][]float64 {
	switch geom.Type {
	case "LineString":
		var line [][]float64
		if json.Unmarshal(geom.Coordinates, &line) == nil && validLine(line) {
			return [][][]float64{line}
		}
	case "MultiLineString":
		var lines [][][]float64
		if json.Unmarshal(geom.Coordinates, &lines) == nil {
			out := make([][][]float64, 0, len(lines))
			for _, line := range lines {
				if validLine(line) {
					out = append(out, line)
				}
			}
			return out
		}
	default:
		for _, rings := range Polygons(geom) {
			if len(rings) > 0 && validLine(rings[0]) {
				return [][][]float64{rings[0]}
			}
		}
	}
	return nil
}

func cleanPolygon(rings [][][]float64) [][][]float64 {
	out := make([][][]float64, 0, len(rings))
	for _, ring := range rings {
		closed := closedRing(ring)
		if len(closed) >= 4 {
			out = append(out, closed)
		}
	}
	return out
}

func alreadyClosed(line [][]float64) bool {
	if !validLine(line) || len(line) < 4 {
		return false
	}
	first, last := line[0], line[len(line)-1]
	return first[0] == last[0] && first[1] == last[1]
}

func closedRing(line [][]float64) [][]float64 {
	if !validLine(line) {
		return nil
	}
	out := append([][]float64{}, line...)
	first, last := out[0], out[len(out)-1]
	if first[0] != last[0] || first[1] != last[1] {
		if len(out) < 3 {
			return nil
		}
		out = append(out, []float64{first[0], first[1]})
	}
	if len(out) < 4 {
		return nil
	}
	return out
}

func validLine(line [][]float64) bool {
	if len(line) < 2 {
		return false
	}
	for _, point := range line {
		if !validPosition(point) {
			return false
		}
	}
	return true
}

func validPosition(point []float64) bool {
	return len(point) >= 2 && point[0] >= -180 && point[0] <= 180 && point[1] >= -90 && point[1] <= 90
}

const earthKm = 6371.0

func DistanceKm(a, b []float64) float64 {
	if len(a) < 2 || len(b) < 2 {
		return 0
	}
	lat1 := a[1] * math.Pi / 180
	lat2 := b[1] * math.Pi / 180
	dLat := lat2 - lat1
	dLon := (b[0] - a[0]) * math.Pi / 180
	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)
	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon
	return 2 * earthKm * math.Asin(math.Min(1, math.Sqrt(h)))
}

// DecimateLine keeps endpoints, drops vertices closer than minKm, then
// downsamples to maxVertices. Rings stay closed.
func DecimateLine(line [][]float64, minKm float64, maxVertices int) [][]float64 {
	if len(line) <= 2 {
		return line
	}
	closed := len(line) >= 4 && line[0][0] == line[len(line)-1][0] && line[0][1] == line[len(line)-1][1]
	body := line
	if closed {
		body = line[:len(line)-1]
	}
	kept := make([][]float64, 0, len(body))
	kept = append(kept, body[0])
	last := body[0]
	for i := 1; i < len(body)-1; i++ {
		if DistanceKm(last, body[i]) >= minKm {
			kept = append(kept, body[i])
			last = body[i]
		}
	}
	kept = append(kept, body[len(body)-1])
	limit := maxVertices
	if closed && limit > 1 {
		limit--
	}
	if limit >= 2 && len(kept) > limit {
		step := float64(len(kept)-1) / float64(limit-1)
		reduced := make([][]float64, 0, limit)
		for i := 0; i < limit-1; i++ {
			reduced = append(reduced, kept[int(float64(i)*step)])
		}
		kept = append(reduced, kept[len(kept)-1])
	}
	if closed {
		kept = append(kept, []float64{kept[0][0], kept[0][1]})
	}
	return kept
}

func DecimateLines(lines [][][]float64, minKm float64, maxVertices int) [][][]float64 {
	out := make([][][]float64, 0, len(lines))
	for _, line := range lines {
		if reduced := DecimateLine(line, minKm, maxVertices); validLine(reduced) {
			out = append(out, reduced)
		}
	}
	return out
}

func DecimatePolygons(polygons [][][][]float64, minKm float64, maxVertices int) [][][][]float64 {
	out := make([][][][]float64, 0, len(polygons))
	for _, rings := range polygons {
		cleaned := make([][][]float64, 0, len(rings))
		for _, ring := range rings {
			reduced := DecimateLine(ring, minKm, maxVertices)
			if closed := closedRing(reduced); len(closed) >= 4 {
				cleaned = append(cleaned, closed)
			}
		}
		if len(cleaned) > 0 {
			out = append(out, cleaned)
		}
	}
	return out
}

// LargestPolygon keeps only the polygon whose exterior ring has the most
// vertices after a first-pass parse — enough for globe-scale fill.
func LargestPolygon(polygons [][][][]float64) [][][]float64 {
	best := [][][]float64{}
	bestN := 0
	for _, rings := range polygons {
		if len(rings) == 0 {
			continue
		}
		if n := len(rings[0]); n > bestN {
			best = rings
			bestN = n
		}
	}
	return best
}
