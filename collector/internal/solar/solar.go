// Package solar computes the subsolar point and a twilight-band polygon.
// The subsolar algorithm matches Unreal's UGeoMathLibrary::SolarSubpoint
// so the collector band sits on the same terminator the globe already lights.
package solar

import "math"

func normalizeDeg(value float64) float64 {
	value = math.Mod(value+180.0, 360.0)
	if value < 0 {
		value += 360.0
	}
	return value - 180.0
}

// Subsolar returns WGS84 latitude/longitude of the subsolar point at utc.
func Subsolar(unixSec int64, nano int) (lat, lon float64) {
	jd := float64(unixSec)/86400.0 + 2440587.5 + float64(nano)/1e9/86400.0
	days := jd - 2451545.0
	meanAnomaly := normalizeDeg(357.529+0.98560028*days) * math.Pi / 180.0
	meanLongitude := normalizeDeg(280.459 + 0.98564736*days)
	ecliptic := normalizeDeg(meanLongitude+1.915*math.Sin(meanAnomaly)+0.020*math.Sin(2*meanAnomaly)) * math.Pi / 180.0
	obliquity := (23.439 - 0.00000036*days) * math.Pi / 180.0
	ra := math.Atan2(math.Cos(obliquity)*math.Sin(ecliptic), math.Cos(ecliptic))
	dec := math.Asin(math.Sin(obliquity) * math.Sin(ecliptic))
	gmst := normalizeDeg(280.46061837 + 360.98564736629*days)
	lat = dec * 180.0 / math.Pi
	lon = normalizeDeg(ra*180.0/math.Pi - gmst)
	return lat, lon
}

func meridianLat(lat0, lon0, alphaDeg, lon float64) (float64, bool) {
	φ0 := lat0 * math.Pi / 180.0
	α := alphaDeg * math.Pi / 180.0
	Δλ := (lon - lon0) * math.Pi / 180.0
	a := math.Sin(φ0)
	b := math.Cos(φ0) * math.Cos(Δλ)
	c := math.Cos(α)
	denom := math.Hypot(a, b)
	if denom < 1e-12 || math.Abs(c) > denom+1e-9 {
		return 0, false
	}
	gamma := math.Atan2(b, a)
	sinArg := clampUnit(c / denom)
	φ := math.Asin(sinArg) - gamma
	lat := φ * 180.0 / math.Pi
	if lat > 90 || lat < -90 || math.IsNaN(lat) {
		return 0, false
	}
	return lat, true
}

func clampUnit(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}

const (
	TerminatorDeg = 90.0
	TwilightDeg   = 102.0
	lonStepDeg    = 5.0
)

// TwilightBand builds a closed GeoJSON ring for the night-side twilight
// strip: terminator along increasing longitude, nautical twilight back.
func TwilightBand(lat0, lon0 float64) [][]float64 {
	inner := make([][]float64, 0, 80)
	outer := make([][]float64, 0, 80)
	for raw := -180.0; raw <= 180.0+1e-9; raw += lonStepDeg {
		lon := clampLon(raw)
		if lat, ok := meridianLat(lat0, lon0, TerminatorDeg, lon); ok {
			inner = append(inner, []float64{lon, lat})
		}
		if lat, ok := meridianLat(lat0, lon0, TwilightDeg, lon); ok {
			outer = append(outer, []float64{lon, lat})
		}
	}
	if len(inner) < 8 || len(outer) < 8 {
		return nil
	}
	ring := make([][]float64, 0, len(inner)+len(outer)+1)
	ring = append(ring, inner...)
	for i := len(outer) - 1; i >= 0; i-- {
		ring = append(ring, outer[i])
	}
	ring = append(ring, []float64{ring[0][0], ring[0][1]})
	return ring
}

func clampLon(lon float64) float64 {
	if lon > 180 {
		return 180
	}
	if lon < -180 {
		return -180
	}
	return lon
}
