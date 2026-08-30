// Package celestrak turns CelesTrak two-line element sets (https://celestrak.org,
// data courtesy of CelesTrak/T.S. Kelso) into live satellite positions via
// SGP4 propagation. TLEs refresh every few hours; positions are emitted on a
// short cadence for every tracked object.
package celestrak

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	satellite "github.com/joshuaferrara/go-satellite"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	defaultURL       = "https://celestrak.org/NORAD/elements/gp.php?GROUP=amateur&FORMAT=tle"
	tleRefresh       = 6 * time.Hour
	positionInterval = 10 * time.Second
	maxSatellites    = 400
	// CelesTrak's usage policy requires machine clients to stop querying on
	// any non-200 response. Without this, a failing fetch left sets empty and
	// the loop refetched every positionInterval forever - exactly the abuse
	// that gets an address firewalled.
	fetchBackoffInitial = 5 * time.Minute
	fetchBackoffMax     = 2 * time.Hour
	fetchFailureLimit   = 8
)

type trackedSatellite struct {
	name  string
	norad string
	sgp4  satellite.Satellite
}

type Source struct {
	id  string
	url string
	// Ground station to compute look angles from. Zero latitude AND
	// longitude means unset - the Gulf of Guinea is not a plausible station
	// and is the conventional sentinel for "no position configured".
	observer observer
	client   *http.Client
	logger   *slog.Logger
	// fetch and now are swappable for tests.
	fetch func(ctx context.Context) ([]byte, error)
	now   func() time.Time
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "orbital.celestrak" {
		return nil, fmt.Errorf("unsupported celestrak source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	source := &Source{id: sourceConfig.ID, url: url, client: &http.Client{Timeout: 30 * time.Second}, logger: logger, now: time.Now}
	if sourceConfig.Latitude != 0 || sourceConfig.Longitude != 0 {
		source.observer = observer{
			latitude:   sourceConfig.Latitude,
			longitude:  sourceConfig.Longitude,
			altitudeKm: sourceConfig.AltitudeM / 1000.0,
			set:        true,
		}
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "orbital.celestrak" }

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", s.url, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 4<<20))
}

// ParseTLESets reads the classic three-line format (name, line1, line2).
func ParseTLESets(body []byte) []trackedSatellite {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	var sets []trackedSatellite
	for index := 0; index+2 < len(lines); index += 3 {
		name := strings.TrimSpace(lines[index])
		line1 := strings.TrimSpace(lines[index+1])
		line2 := strings.TrimSpace(lines[index+2])
		if name == "" || !strings.HasPrefix(line1, "1 ") || !strings.HasPrefix(line2, "2 ") {
			continue
		}
		if len(sets) >= maxSatellites {
			break
		}
		sets = append(sets, trackedSatellite{
			name:  name,
			norad: strings.Fields(line2)[1],
			sgp4:  satellite.TLEToSat(line1, line2, satellite.GravityWGS84),
		})
	}
	return sets
}

// observer is the ground station look angles are measured from.
type observer struct {
	latitude   float64
	longitude  float64
	altitudeKm float64
	set        bool
}

// lookAngles returns azimuth and elevation in degrees, plus slant range in
// km, for a satellite at an ECI position and time. Elevation is negative when
// the satellite is below the horizon, which is what makes "is it up now"
// answerable without a second propagation.
func (o observer) lookAngles(position satellite.Vector3, at time.Time) (azimuthDeg, elevationDeg, rangeKm float64) {
	jday := satellite.JDay(at.Year(), int(at.Month()), at.Day(), at.Hour(), at.Minute(), at.Second())
	angles := satellite.ECIToLookAngles(position, satellite.LatLong{
		Latitude:  o.latitude * math.Pi / 180.0,
		Longitude: o.longitude * math.Pi / 180.0,
	}, o.altitudeKm, jday)
	return angles.Az * 180.0 / math.Pi, angles.El * 180.0 / math.Pi, angles.Rg
}

// PositionRecord propagates one satellite to the given time.
func PositionRecord(tracked trackedSatellite, at time.Time, sourceInstanceID string) (plugins.RawRecord, bool) {
	return positionRecordFrom(tracked, at, sourceInstanceID, observer{})
}

func positionRecordFrom(tracked trackedSatellite, at time.Time, sourceInstanceID string, from observer) (plugins.RawRecord, bool) {
	at = at.UTC()
	position, _ := satellite.Propagate(tracked.sgp4, at.Year(), int(at.Month()), at.Day(), at.Hour(), at.Minute(), at.Second())
	if math.IsNaN(position.X) || (position.X == 0 && position.Y == 0 && position.Z == 0) {
		return plugins.RawRecord{}, false
	}
	gmst := satellite.GSTimeFromDate(at.Year(), int(at.Month()), at.Day(), at.Hour(), at.Minute(), at.Second())
	altitudeKm, _, radians := satellite.ECIToLLA(position, gmst)
	degrees := satellite.LatLongDeg(radians)
	longitude := math.Mod(degrees.Longitude+540.0, 360.0) - 180.0
	if math.IsNaN(degrees.Latitude) || math.IsNaN(longitude) || math.IsNaN(altitudeKm) || altitudeKm < 100 || altitudeKm > 100000 {
		return plugins.RawRecord{}, false
	}
	payload := fmt.Sprintf(`{"satId":"%s","name":%q,"latitude":%.5f,"longitude":%.5f,"altKm":%.1f}`,
		tracked.norad, tracked.name, degrees.Latitude, longitude, altitudeKm)
	if from.set {
		azimuth, elevation, slantRange := from.lookAngles(position, at)
		payload = fmt.Sprintf(`{"satId":"%s","name":%q,"latitude":%.5f,"longitude":%.5f,"altKm":%.1f,"azDeg":%.1f,"elDeg":%.1f,"rangeKm":%.0f}`,
			tracked.norad, tracked.name, degrees.Latitude, longitude, altitudeKm, azimuth, elevation, slantRange)
	}
	return plugins.RawRecord{
		SourcePluginID:   "celestrak",
		SourceInstanceID: sourceInstanceID,
		OriginalID:       fmt.Sprintf("sat-%s-%s-%d", sourceInstanceID, tracked.norad, at.Unix()),
		Domain:           "orbital",
		ObservedUTC:      at,
		Payload:          []byte(payload),
	}, true
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	var sets []trackedSatellite
	var lastRefresh time.Time
	// Retry governor: a failed or empty fetch must not turn the position
	// ticker into a download loop (CelesTrak usage policy).
	var consecutiveFailures int
	var nextFetchAllowed time.Time
	// Pass prediction runs on its own, much slower cadence: a sweep is a few
	// hundred thousand propagations, and the answer does not change between
	// position ticks.
	var nextPassSweep time.Time
	var predictions []predictedPass
	ticker := time.NewTicker(positionInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		dueForRefresh := len(sets) == 0 || time.Since(lastRefresh) > tleRefresh
		if dueForRefresh && consecutiveFailures < fetchFailureLimit && time.Now().After(nextFetchAllowed) {
			body, err := s.fetch(ctx)
			parsed := []trackedSatellite(nil)
			if err == nil {
				parsed = ParseTLESets(body)
			}
			if err == nil && len(parsed) > 0 {
				s.storeTLECache(body)
			} else if len(sets) == 0 {
				// Nothing in memory and the service is unreachable. Elements
				// change slowly, so yesterday's set still predicts today's
				// passes to within a second or two - far better than showing
				// nothing at all because a server is down or has firewalled
				// us. Only ever used as a fallback, never in place of a
				// successful fetch.
				if cached, age, ok := s.loadTLECache(); ok {
					if fromCache := ParseTLESets(cached); len(fromCache) > 0 {
						sets = fromCache
						lastRefresh = s.now()
						s.logger.Warn("celestrak unreachable; using cached TLE set",
							"satellites", len(sets), "ageHours", int(age.Hours()))
					}
				}
			}
			switch {
			case err != nil || len(parsed) == 0:
				consecutiveFailures++
				backoff := fetchBackoffInitial << min(consecutiveFailures-1, 8)
				if backoff > fetchBackoffMax || backoff <= 0 {
					backoff = fetchBackoffMax
				}
				nextFetchAllowed = time.Now().Add(backoff)
				reason := "empty or unparsable TLE response"
				if err != nil {
					reason = err.Error()
				}
				s.logger.Warn("celestrak TLE fetch failed; backing off",
					"error", reason, "consecutiveFailures", consecutiveFailures, "retryIn", backoff.String())
				if consecutiveFailures >= fetchFailureLimit {
					s.logger.Error("celestrak fetch giving up until restart; refusing to keep polling (usage policy)",
						"consecutiveFailures", consecutiveFailures)
				}
			default:
				if consecutiveFailures > 0 {
					s.logger.Info("celestrak recovered", "afterFailures", consecutiveFailures)
				}
				consecutiveFailures = 0
				nextFetchAllowed = time.Time{}
				sets = parsed
				lastRefresh = time.Now()
				s.logger.Info("celestrak TLE set refreshed", "satellites", len(sets))
			}
		}
		at := s.now()
		for _, tracked := range sets {
			record, ok := positionRecordFrom(tracked, at, s.id, s.observer)
			if !ok {
				continue
			}
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		if s.observer.set && len(sets) > 0 && at.After(nextPassSweep) {
			nextPassSweep = at.Add(passRecomputeInterval)
			predictions = predictions[:0]
			for _, tracked := range sets {
				if ctx.Err() != nil {
					return nil
				}
				pass, ok := s.observer.nextPass(tracked, at)
				if !ok {
					continue
				}
				predictions = append(predictions, predictedPass{tracked: tracked, pass: pass})
			}
			s.logger.Info("celestrak pass prediction swept",
				"satellites", len(sets), "passesWithin24h", len(predictions),
				"station", fmt.Sprintf("%.4f,%.4f", s.observer.latitude, s.observer.longitude))
		}
		// Re-announce the cached predictions on every tick. There is no
		// state snapshot on connect, so a client that joins between sweeps
		// would otherwise sit on "awaiting prediction" for up to the whole
		// recompute interval. Re-emitting is nearly free - the expensive
		// part was the search, not the record - and a repeated envelope for
		// the same pass supersedes rather than accumulates, because the id
		// is keyed on the acquisition time.
		for _, predicted := range predictions {
			if predicted.pass.loss.Before(at) {
				continue
			}
			select {
			case output <- PassRecord(predicted.tracked, predicted.pass, s.id, at):
			case <-ctx.Done():
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
	return nil
}
