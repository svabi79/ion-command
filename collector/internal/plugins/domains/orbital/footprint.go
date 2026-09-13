package orbital

import "math"

const (
	earthRadiusKm     = 6371.0088
	footprintVertices = 24
	minFootprintAltKm = 150
	maxFootprintAltKm = 50000
)

// visibilityRadiusDeg is the Earth-central angle from nadir to the
// geometric radio horizon (0° elevation). Pure sphere; no refraction.
func visibilityRadiusDeg(altKm float64) (float64, bool) {
	if altKm < minFootprintAltKm || altKm > maxFootprintAltKm || math.IsNaN(altKm) {
		return 0, false
	}
	cosGamma := earthRadiusKm / (earthRadiusKm + altKm)
	if cosGamma <= 0 || cosGamma >= 1 {
		return 0, false
	}
	return math.Acos(cosGamma) * 180.0 / math.Pi, true
}

// visibilityRing returns a closed GeoJSON ring [lon, lat] around nadir.
// Longitudes stay in [-180, 180]. The circle is approximate (spherical
// destination) — globe-scale radio/visibility, not a regulatory contour.
func visibilityRing(lat, lon, altKm float64, vertices int) ([][]float64, bool) {
	radiusDeg, ok := visibilityRadiusDeg(altKm)
	if !ok || vertices < 8 {
		return nil, false
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, false
	}
	ring := make([][]float64, 0, vertices+1)
	for i := 0; i < vertices; i++ {
		bearing := 360.0 * float64(i) / float64(vertices)
		dLat, dLon := destination(lat, lon, bearing, radiusDeg)
		ring = append(ring, []float64{dLon, dLat})
	}
	ring = append(ring, []float64{ring[0][0], ring[0][1]})
	return ring, true
}

func destination(lat, lon, bearingDeg, distanceDeg float64) (float64, float64) {
	φ1 := lat * math.Pi / 180.0
	λ1 := lon * math.Pi / 180.0
	θ := bearingDeg * math.Pi / 180.0
	δ := distanceDeg * math.Pi / 180.0
	sinφ1, cosφ1 := math.Sincos(φ1)
	sinδ, cosδ := math.Sincos(δ)
	sinθ, cosθ := math.Sincos(θ)
	sinφ2 := sinφ1*cosδ + cosφ1*sinδ*cosθ
	φ2 := math.Asin(clampUnit(sinφ2))
	y := sinθ * sinδ * cosφ1
	x := cosδ - sinφ1*sinφ2
	λ2 := λ1 + math.Atan2(y, x)
	return clampLat(φ2 * 180.0 / math.Pi), wrapLon(λ2 * 180.0 / math.Pi)
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

func clampLat(lat float64) float64 {
	if lat > 90 {
		return 90
	}
	if lat < -90 {
		return -90
	}
	return lat
}

func wrapLon(lon float64) float64 {
	lon = math.Mod(lon+180.0, 360.0)
	if lon < 0 {
		lon += 360.0
	}
	return lon - 180.0
}
