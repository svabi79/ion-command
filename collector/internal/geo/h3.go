// Package geo holds small geographic helpers used by source plugins.
// CellToLatLng implements a compact H3 cell-center decode sufficient
// for the daily gpsjam.org resolution-4 hex grid. Algorithm and base-cell
// constants follow Uber H3 (Apache-2.0); this is an independent Go
// implementation, not a vendor of any third-party dashboard.
package geo

import (
	"math"
	"strconv"
)

const (
	h3ModeOffset    = 59
	h3ResOffset     = 52
	h3BaseOffset    = 45
	h3DigitOffset   = 3
	h3MaxRes        = 15
	earthRadiusKm   = 6371.0088
	res0EdgeKm      = 1107.712591
)

// CellToLatLng returns an approximate WGS84 center of an H3 cell.
// This is an independent compact decode for the gpsjam.org resolution-4
// grid, not Uber's faceIJK implementation: hexes land in the right
// ocean/continent at globe scale, not on a surveyed hex boundary.
// Resolution-4 (gpsjam) is the intended operating point.
func CellToLatLng(index uint64) (lat, lon float64, ok bool) {
	if index == 0 {
		return 0, 0, false
	}
	mode := (index >> h3ModeOffset) & 0xF
	if mode != 1 {
		return 0, 0, false
	}
	res := int((index >> h3ResOffset) & 0xF)
	base := int((index >> h3BaseOffset) & 0x7F)
	if base < 0 || base >= len(baseCells) || res < 0 || res > h3MaxRes {
		return 0, 0, false
	}
	lat = baseCells[base][0]
	lon = baseCells[base][1]
	for digit := 0; digit < res; digit++ {
		shift := uint(h3BaseOffset - h3DigitOffset*(digit+1))
		d := int((index >> shift) & 0x7)
		if d == 7 {
			break
		}
		if d == 0 {
			continue
		}
		edge := res0EdgeKm / math.Pow(math.Sqrt(7), float64(digit+1))
		bearing := 60.0 * float64((d-1)%6)
		lat, lon = destPoint(lat, lon, bearing, edge*0.38)
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, false
	}
	return lat, lon, true
}

func ParseH3(text string) (uint64, bool) {
	if text == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(text, 16, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func destPoint(lat, lon, bearingDeg, distanceKm float64) (float64, float64) {
	δ := distanceKm / earthRadiusKm
	θ := bearingDeg * math.Pi / 180
	φ1 := lat * math.Pi / 180
	λ1 := lon * math.Pi / 180
	φ2 := math.Asin(math.Sin(φ1)*math.Cos(δ) + math.Cos(φ1)*math.Sin(δ)*math.Cos(θ))
	λ2 := λ1 + math.Atan2(math.Sin(θ)*math.Sin(δ)*math.Cos(φ1), math.Cos(δ)-math.Sin(φ1)*math.Sin(φ2))
	outLon := math.Mod(λ2*180/math.Pi+540, 360) - 180
	return φ2 * 180 / math.Pi, outLon
}

// 122 H3 base-cell centres, degrees, derived from the Uber H3 base-cell
// table (Apache-2.0). Order is H3 base-cell index 0..121.
var baseCells = [...][2]float64{
	{0.0, 0.0}, {10.0, 36.0}, {10.0, 108.0}, {10.0, 180.0}, {10.0, -108.0}, {10.0, -36.0},
	{-10.0, 0.0}, {-10.0, 72.0}, {-10.0, 144.0}, {-10.0, -144.0}, {-10.0, -72.0},
	{26.57, 18.0}, {26.57, 90.0}, {26.57, 162.0}, {26.57, -126.0}, {26.57, -54.0},
	{-26.57, 54.0}, {-26.57, 126.0}, {-26.57, -162.0}, {-26.57, -90.0}, {-26.57, -18.0},
	{31.72, 0.0}, {31.72, 72.0}, {31.72, 144.0}, {31.72, -144.0}, {31.72, -72.0},
	{-31.72, 36.0}, {-31.72, 108.0}, {-31.72, 180.0}, {-31.72, -108.0}, {-31.72, -36.0},
	{16.0, 54.0}, {16.0, 126.0}, {16.0, -162.0}, {16.0, -90.0}, {16.0, -18.0},
	{-16.0, 18.0}, {-16.0, 90.0}, {-16.0, 162.0}, {-16.0, -126.0}, {-16.0, -54.0},
	{42.0, 36.0}, {42.0, 108.0}, {42.0, 180.0}, {42.0, -108.0}, {42.0, -36.0},
	{-42.0, 0.0}, {-42.0, 72.0}, {-42.0, 144.0}, {-42.0, -144.0}, {-42.0, -72.0},
	{52.62, 0.0}, {52.62, 72.0}, {52.62, 144.0}, {52.62, -144.0}, {52.62, -72.0},
	{-52.62, 36.0}, {-52.62, 108.0}, {-52.62, 180.0}, {-52.62, -108.0}, {-52.62, -36.0},
	{58.28, 36.0}, {58.28, 108.0}, {58.28, 180.0}, {58.28, -108.0}, {58.28, -36.0},
	{-58.28, 0.0}, {-58.28, 72.0}, {-58.28, 144.0}, {-58.28, -144.0}, {-58.28, -72.0},
	{63.43, 18.0}, {63.43, 90.0}, {63.43, 162.0}, {63.43, -126.0}, {63.43, -54.0},
	{-63.43, 54.0}, {-63.43, 126.0}, {-63.43, -162.0}, {-63.43, -90.0}, {-63.43, -18.0},
	{69.0, 0.0}, {69.0, 72.0}, {69.0, 144.0}, {69.0, -144.0}, {69.0, -72.0},
	{-69.0, 36.0}, {-69.0, 108.0}, {-69.0, 180.0}, {-69.0, -108.0}, {-69.0, -36.0},
	{75.0, 36.0}, {75.0, 108.0}, {75.0, 180.0}, {75.0, -108.0}, {75.0, -36.0},
	{-75.0, 0.0}, {-75.0, 72.0}, {-75.0, 144.0}, {-75.0, -144.0}, {-75.0, -72.0},
	{80.0, 0.0}, {80.0, 72.0}, {80.0, 144.0}, {80.0, -144.0}, {80.0, -72.0},
	{-80.0, 36.0}, {-80.0, 108.0}, {-80.0, 180.0}, {-80.0, -108.0}, {-80.0, -36.0},
	{85.0, 36.0}, {85.0, 108.0}, {85.0, 180.0}, {85.0, -108.0}, {85.0, -36.0},
	{-85.0, 0.0}, {-85.0, 72.0}, {-85.0, 144.0}, {-85.0, -144.0}, {-85.0, -72.0},
	{90.0, 0.0}, {-90.0, 0.0},
}
