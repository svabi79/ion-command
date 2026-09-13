package nhc

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const maxRingVertices = 256

var coordinatesPattern = regexp.MustCompile(`(?is)<coordinates[^>]*>(.*?)</coordinates>`)

func parseConeGeometry(body []byte) ([][][]float64, error) {
	payload := body
	if len(body) >= 2 && body[0] == 'P' && body[1] == 'K' {
		kml, err := unzipFirstKML(body)
		if err != nil {
			return nil, err
		}
		payload = kml
	}
	rings, err := parseKMLCoordinates(payload)
	if err != nil {
		return nil, err
	}
	out := make([][][]float64, 0, len(rings))
	for _, ring := range rings {
		if simplified := downsampleRing(ring, maxRingVertices); len(simplified) >= 3 {
			out = append(out, simplified)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cone geometry has no usable ring")
	}
	return out, nil
}

func unzipFirstKML(body []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("decode cone kmz: %w", err)
	}
	for _, file := range reader.File {
		name := strings.ToLower(file.Name)
		if !strings.HasSuffix(name, ".kml") {
			continue
		}
		handle, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open cone kml: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(handle, 4<<20))
		handle.Close()
		if err != nil {
			return nil, fmt.Errorf("read cone kml: %w", err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("cone kmz has no kml")
}

func parseKMLCoordinates(body []byte) ([][][]float64, error) {
	matches := coordinatesPattern.FindAllSubmatch(body, 8)
	if len(matches) == 0 {
		return nil, fmt.Errorf("kml has no coordinates")
	}
	rings := make([][][]float64, 0, len(matches))
	for _, match := range matches {
		ring := parseCoordinateText(string(match[1]))
		if len(ring) >= 3 {
			rings = append(rings, closeRing(ring))
		}
	}
	if len(rings) == 0 {
		return nil, fmt.Errorf("kml coordinates were empty")
	}
	return rings, nil
}

func parseCoordinateText(text string) [][]float64 {
	fields := strings.Fields(strings.ReplaceAll(text, "\n", " "))
	ring := make([][]float64, 0, len(fields))
	for _, field := range fields {
		parts := strings.Split(field, ",")
		if len(parts) < 2 {
			continue
		}
		lon, errLon := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errLon != nil || errLat != nil {
			continue
		}
		if lon < -180 || lon > 180 || lat < -90 || lat > 90 {
			continue
		}
		ring = append(ring, []float64{lon, lat})
	}
	return ring
}

func samePoint(a, b []float64) bool {
	return len(a) >= 2 && len(b) >= 2 && a[0] == b[0] && a[1] == b[1]
}

func closeRing(ring [][]float64) [][]float64 {
	if len(ring) == 0 {
		return ring
	}
	if !samePoint(ring[0], ring[len(ring)-1]) {
		return append(ring, []float64{ring[0][0], ring[0][1]})
	}
	return ring
}

func downsampleRing(ring [][]float64, maxVertices int) [][]float64 {
	if maxVertices < 4 {
		maxVertices = 4
	}
	ring = closeRing(ring)
	if len(ring) <= maxVertices {
		return ring
	}
	body := ring[:len(ring)-1]
	kept := maxVertices - 1
	if kept < 3 {
		kept = 3
	}
	out := make([][]float64, 0, kept+1)
	step := float64(len(body)) / float64(kept)
	for i := 0; i < kept; i++ {
		out = append(out, body[int(float64(i)*step)])
	}
	return closeRing(out)
}
