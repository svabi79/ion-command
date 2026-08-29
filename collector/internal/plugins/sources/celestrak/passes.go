package celestrak

import (
	"fmt"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
	satellite "github.com/joshuaferrara/go-satellite"
)

const (
	// How far ahead to look. A day covers every amateur satellite's repeat
	// cycle, so "no pass in this window" genuinely means "not today".
	passHorizon = 24 * time.Hour
	// Coarse search step. Amateur satellites in low orbit stay above the
	// horizon for roughly 5-15 minutes, so a minute cannot step over a pass;
	// the refinement below recovers the exact edges.
	passCoarseStep = time.Minute
	// Refinement step for the horizon crossings and the culmination.
	passFineStep = 5 * time.Second
	// Passes that never clear this are not worth announcing: below it the
	// satellite is in the ground clutter of any real station.
	passMinPeakElevation = 10.0
	// How often the whole catalogue is re-predicted. Each sweep is hundreds
	// of thousands of propagations, which is cheap but not free.
	passRecomputeInterval = 10 * time.Minute
)

// predictedPass pairs a satellite with its next appearance, so the sweep's
// result can be re-announced without re-searching for it.
type predictedPass struct {
	tracked trackedSatellite
	pass    satellitePass
}

// satellitePass is one horizon-to-horizon appearance over the station.
type satellitePass struct {
	acquisition time.Time
	culmination time.Time
	loss        time.Time
	peakDegrees float64
	// Azimuth at acquisition and loss: where to point first, and where it
	// will go. Without these a pass time says when but not where.
	acquisitionAzimuth float64
	lossAzimuth        float64
}

func (p satellitePass) duration() time.Duration { return p.loss.Sub(p.acquisition) }

// elevationAt is the satellite's elevation over the observer, in degrees.
func (o observer) elevationAt(tracked trackedSatellite, at time.Time) (elevation, azimuth float64, ok bool) {
	at = at.UTC()
	position, _ := satellite.Propagate(tracked.sgp4, at.Year(), int(at.Month()), at.Day(), at.Hour(), at.Minute(), at.Second())
	if position.X == 0 && position.Y == 0 && position.Z == 0 {
		return 0, 0, false
	}
	azimuthDeg, elevationDeg, _ := o.lookAngles(position, at)
	return elevationDeg, azimuthDeg, true
}

// nextPass finds the next appearance over the horizon after from, or reports
// that there is none inside the horizon window.
//
// The search is deliberately a scan rather than an analytic solution: SGP4 is
// cheap, the window is bounded, and a scan cannot be wrong about a geometry
// an analytic shortcut mis-models. Coarse steps find the pass, fine steps
// pin its edges.
func (o observer) nextPass(tracked trackedSatellite, from time.Time) (satellitePass, bool) {
	if !o.set {
		return satellitePass{}, false
	}
	deadline := from.Add(passHorizon)
	previousElevation, _, ok := o.elevationAt(tracked, from)
	if !ok {
		return satellitePass{}, false
	}
	// Starting mid-pass would report an acquisition that already happened.
	// Wait for the satellite to set before looking for the next rise.
	waitingForSet := previousElevation > 0

	// Iterative rather than recursive: a satellite can graze the horizon
	// many times in a day, and each rejected pass would be another stack
	// frame.
	for at := from.Add(passCoarseStep); at.Before(deadline); at = at.Add(passCoarseStep) {
		elevation, _, ok := o.elevationAt(tracked, at)
		if !ok {
			return satellitePass{}, false
		}
		if waitingForSet {
			if elevation <= 0 {
				waitingForSet = false
			}
			previousElevation = elevation
			continue
		}
		if previousElevation <= 0 && elevation > 0 {
			pass, complete := o.refinePass(tracked, at.Add(-passCoarseStep), deadline)
			if !complete {
				return satellitePass{}, false
			}
			if pass.peakDegrees >= passMinPeakElevation {
				return pass, true
			}
			// Too low to announce. Resume the scan just after it set.
			at = pass.loss
			previousElevation = 0
			continue
		}
		previousElevation = elevation
	}
	return satellitePass{}, false
}

// refinePass walks forward from just below the horizon to find the exact
// rise, the culmination and the set.
func (o observer) refinePass(tracked trackedSatellite, justBelowHorizon time.Time, deadline time.Time) (satellitePass, bool) {
	pass := satellitePass{peakDegrees: -90}
	found := false
	for at := justBelowHorizon; at.Before(deadline); at = at.Add(passFineStep) {
		elevation, azimuth, ok := o.elevationAt(tracked, at)
		if !ok {
			return satellitePass{}, false
		}
		if !found {
			if elevation <= 0 {
				continue
			}
			pass.acquisition = at
			pass.acquisitionAzimuth = azimuth
			found = true
		}
		if elevation > pass.peakDegrees {
			pass.peakDegrees = elevation
			pass.culmination = at
		}
		if found && elevation <= 0 {
			pass.loss = at
			pass.lossAzimuth = azimuth
			return pass, true
		}
	}
	return satellitePass{}, false
}

// PassRecord renders a predicted pass as a raw record.
func PassRecord(tracked trackedSatellite, pass satellitePass, sourceInstanceID string, at time.Time) plugins.RawRecord {
	payload := fmt.Sprintf(
		`{"kind":"pass","satId":"%s","name":%q,"aosUtc":"%s","tcaUtc":"%s","losUtc":"%s","peakElDeg":%.1f,"aosAzDeg":%.0f,"losAzDeg":%.0f,"durationS":%.0f}`,
		tracked.norad, tracked.name,
		pass.acquisition.UTC().Format(time.RFC3339),
		pass.culmination.UTC().Format(time.RFC3339),
		pass.loss.UTC().Format(time.RFC3339),
		pass.peakDegrees, pass.acquisitionAzimuth, pass.lossAzimuth,
		pass.duration().Seconds())
	return plugins.RawRecord{
		SourcePluginID:   "celestrak",
		SourceInstanceID: sourceInstanceID,
		// One record per satellite per predicted pass: a re-run that predicts
		// the same pass replaces it rather than piling up duplicates.
		OriginalID:  fmt.Sprintf("pass-%s-%s-%d", sourceInstanceID, tracked.norad, pass.acquisition.Unix()),
		Domain:      "orbital",
		ObservedUTC: at,
		Payload:     []byte(payload),
	}
}
