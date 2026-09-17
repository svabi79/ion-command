package glofas

import (
	"encoding/binary"
	"fmt"
	"math"
)

func polygonsFromSHP(shp []byte) ([][][][]float64, error) {
	if len(shp) < 100 {
		return nil, fmt.Errorf("shapefile too small")
	}
	if binary.BigEndian.Uint32(shp[0:4]) != 9994 {
		return nil, fmt.Errorf("not a shapefile")
	}
	out := make([][][][]float64, 0)
	off := 100
	for off+8 <= len(shp) {
		contentWords := int(binary.BigEndian.Uint32(shp[off+4 : off+8]))
		recLen := contentWords * 2
		start := off + 8
		next := off + 8 + recLen
		if start+4 > len(shp) || recLen < 4 {
			break
		}
		shapeType := int32(binary.LittleEndian.Uint32(shp[start : start+4]))
		off = next
		if shapeType != 5 && shapeType != 15 && shapeType != 25 {
			continue
		}
		cursor := start + 4 + 32
		if cursor+8 > len(shp) {
			continue
		}
		numParts := int(int32(binary.LittleEndian.Uint32(shp[cursor : cursor+4])))
		numPoints := int(int32(binary.LittleEndian.Uint32(shp[cursor+4 : cursor+8])))
		cursor += 8
		if numParts <= 0 || numPoints < 3 || numParts > 250000 || numPoints > 2000000 {
			continue
		}
		if cursor+4*numParts > len(shp) {
			continue
		}
		parts := make([]int, numParts)
		for i := 0; i < numParts; i++ {
			parts[i] = int(int32(binary.LittleEndian.Uint32(shp[cursor : cursor+4])))
			cursor += 4
		}
		if cursor+16*numPoints > len(shp) {
			continue
		}
		points := make([][]float64, numPoints)
		for i := 0; i < numPoints; i++ {
			lon := math.Float64frombits(binary.LittleEndian.Uint64(shp[cursor : cursor+8]))
			lat := math.Float64frombits(binary.LittleEndian.Uint64(shp[cursor+8 : cursor+16]))
			cursor += 16
			points[i] = []float64{lon, lat}
		}
		for i := 0; i < numParts; i++ {
			from := parts[i]
			to := numPoints
			if i+1 < numParts {
				to = parts[i+1]
			}
			if from < 0 || to > numPoints || to-from < 3 {
				continue
			}
			ring := append([][]float64{}, points[from:to]...)
			out = append(out, [][][]float64{ring})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("shapefile had no polygon rings")
	}
	return out, nil
}
