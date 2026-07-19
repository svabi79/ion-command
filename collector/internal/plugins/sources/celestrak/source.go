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
)

type trackedSatellite struct {
	name    string
	norad   string
	sgp4    satellite.Satellite
}

type Source struct {
	id     string
	url    string
	client *http.Client
	logger *slog.Logger
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

// PositionRecord propagates one satellite to the given time.
func PositionRecord(tracked trackedSatellite, at time.Time, sourceInstanceID string) (plugins.RawRecord, bool) {
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
	ticker := time.NewTicker(positionInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if len(sets) == 0 || time.Since(lastRefresh) > tleRefresh {
			body, err := s.fetch(ctx)
			if err != nil {
				s.logger.Warn("celestrak TLE fetch failed", "error", err)
			} else if parsed := ParseTLESets(body); len(parsed) > 0 {
				sets = parsed
				lastRefresh = time.Now()
				s.logger.Info("celestrak TLE set refreshed", "satellites", len(sets))
			}
		}
		at := s.now()
		for _, tracked := range sets {
			record, ok := PositionRecord(tracked, at, s.id)
			if !ok {
				continue
			}
			select {
			case output <- record:
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
