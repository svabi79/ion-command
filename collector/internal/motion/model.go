// Package motion is the collector-side course model used by headingprobe
// and tests. The Unreal marker layer applies the same thresholds in C++.
//
// Course rules (issue #7):
//   - below 3 kt, hold the previous display course (hover noise)
//   - below 30 kt, prefer the reported track
//   - above 50 kt, prefer the chord between fixes
//   - blend linearly in between
//   - ignore turn-rate estimates below 10 kt
//   - slew cap starts at 12 deg/s at low speed
//   - coast is bounded by last contact: 60 s grace, 300 s hard maximum
package motion

import "math"

const (
	HoverMPS      = 1.5  // 3 kt
	TrackOnlyMPS  = 15.4 // 30 kt
	ChordMPS      = 25.7 // 50 kt
	TurnIgnoreMPS = 5.14 // 10 kt
	SlewLowDegS   = 12.0
	SlewCruiseDegS = 18.0
	GraceSeconds  = 60.0
	HardCoastSeconds = 300.0
)

func NormalizeDeg(deg float64) float64 {
	for deg < 0 {
		deg += 360
	}
	for deg >= 360 {
		deg -= 360
	}
	return deg
}

func DeltaDeg(from, to float64) float64 {
	delta := NormalizeDeg(to) - NormalizeDeg(from)
	if delta > 180 {
		delta -= 360
	}
	if delta < -180 {
		delta += 360
	}
	return delta
}

func BlendCourse(reportedTrack, chordTrack, speedMPS, previousDisplay float64) float64 {
	if speedMPS < HoverMPS {
		if previousDisplay != 0 {
			return previousDisplay
		}
		return reportedTrack
	}
	if speedMPS <= TrackOnlyMPS {
		return reportedTrack
	}
	if speedMPS >= ChordMPS {
		return chordTrack
	}
	t := (speedMPS - TrackOnlyMPS) / (ChordMPS - TrackOnlyMPS)
	return NormalizeDeg(reportedTrack + DeltaDeg(reportedTrack, chordTrack)*t)
}

func SlewCapDegS(speedMPS float64) float64 {
	if speedMPS <= TrackOnlyMPS {
		return SlewLowDegS
	}
	if speedMPS >= ChordMPS {
		return SlewCruiseDegS
	}
	t := (speedMPS - TrackOnlyMPS) / (ChordMPS - TrackOnlyMPS)
	return SlewLowDegS + (SlewCruiseDegS-SlewLowDegS)*t
}

func ApplySlew(previous, desired, speedMPS, dtSeconds float64) float64 {
	if dtSeconds <= 0 {
		return previous
	}
	cap := SlewCapDegS(speedMPS) * dtSeconds
	delta := DeltaDeg(previous, desired)
	if math.Abs(delta) <= cap {
		return NormalizeDeg(desired)
	}
	if delta < 0 {
		cap = -cap
	}
	return NormalizeDeg(previous + cap)
}

func TurnRateDegS(previousTrack, nextTrack, speedMPS, dtSeconds float64) float64 {
	if speedMPS < TurnIgnoreMPS || dtSeconds <= 0 {
		return 0
	}
	return DeltaDeg(previousTrack, nextTrack) / dtSeconds
}

func CoastSeconds(ageSinceContact float64) float64 {
	if ageSinceContact < 0 {
		ageSinceContact = 0
	}
	if ageSinceContact > HardCoastSeconds {
		return 0
	}
	remain := GraceSeconds - ageSinceContact
	if remain < 0 {
		remain = 0
	}
	hard := HardCoastSeconds - ageSinceContact
	if remain > hard {
		return hard
	}
	return remain
}
