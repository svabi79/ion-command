// Package opensky polls the OpenSky Network global state snapshot
// (https://opensky-network.org/api/states/all). One request returns every
// tracked aircraft worldwide, which complements the fast regional adsb.lol
// point queries with planet-wide ambient coverage. Anonymous access is
// credit-limited (~100 global requests per day), hence the slow default
// poll; configuring login/password raises the budget.
package opensky

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	defaultBase    = "https://opensky-network.org"
	pollDefault    = 15 * time.Minute
	pollFloor      = 5 * time.Minute
	backoffInitial = 2 * time.Minute
	backoffMax     = 30 * time.Minute
	// A state vector counts as live when its position fix is at most this
	// old at fetch time; the snapshot carries hours-stale entries too.
	positionMaxAge = 120 * time.Second
)

// kindForCategory maps the OpenSky aircraft category enum to the generic
// airframe kind vocabulary shared with the readsb-based sources.
func kindForCategory(category int) string {
	switch category {
	case 8:
		return "helicopter"
	case 9, 12:
		return "glider"
	case 10:
		return "balloon"
	case 14:
		return "drone"
	default:
		return "aircraft"
	}
}

type Source struct {
	id       string
	base     string
	login    string
	password string
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger
	// fetch is swappable for tests.
	fetch func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "aviation.opensky" {
		return nil, fmt.Errorf("unsupported opensky source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("opensky poll interval below five minutes would exhaust the anonymous credit budget (got %s)", interval)
	}
	source := &Source{
		id:       sourceConfig.ID,
		base:     defaultBase,
		login:    sourceConfig.Login,
		password: sourceConfig.Password,
		interval: interval,
		client:   &http.Client{Timeout: 90 * time.Second},
		logger:   logger,
	}
	if sourceConfig.Broker != "" {
		source.base = strings.TrimSuffix(sourceConfig.Broker, "/")
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.opensky" }

type errRateLimited struct{ retryAfter time.Duration }

func (e errRateLimited) Error() string {
	return fmt.Sprintf("rate limited by opensky (retry after %s)", e.retryAfter)
}

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	url := s.base + "/api/states/all?extended=1"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	if s.login != "" {
		request.SetBasicAuth(s.login, s.password)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == 420 {
		retryAfter := backoffInitial
		if header := response.Header.Get("X-Rate-Limit-Retry-After-Seconds"); header != "" {
			if seconds, parseErr := strconv.Atoi(strings.TrimSpace(header)); parseErr == nil && seconds > 0 {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
		return nil, errRateLimited{retryAfter: retryAfter}
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", url, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 64<<20))
}

type statesResponse struct {
	Time   int64               `json:"time"`
	States [][]json.RawMessage `json:"states"`
}

func rawString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func rawFloat(raw json.RawMessage) (float64, bool) {
	var value float64
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	return 0, false
}

func rawBool(raw json.RawMessage) bool {
	var value bool
	_ = json.Unmarshal(raw, &value)
	return value
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var response statesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode opensky response: %w", err)
	}
	now := time.Now().UTC()
	snapshot := response.Time
	// Markers must outlive the slow poll cycle plus request jitter.
	validSeconds := int(s.interval/time.Second) + 180
	records := make([]plugins.RawRecord, 0, len(response.States))
	for _, state := range response.States {
		// State vector layout (extended=1): 0 icao24, 1 callsign,
		// 2 origin_country, 3 time_position, 4 last_contact, 5 lon, 6 lat,
		// 7 baro_altitude m, 8 on_ground, 9 velocity m/s, 10 true_track,
		// 11 vertical_rate m/s, 13 geo_altitude, 14 squawk, 17 category.
		if len(state) < 17 {
			continue
		}
		hex := rawString(state[0])
		lon, lonOK := rawFloat(state[5])
		lat, latOK := rawFloat(state[6])
		if hex == "" || !lonOK || !latOK {
			continue
		}
		positionTime, timeOK := rawFloat(state[3])
		if !timeOK || snapshot-int64(positionTime) > int64(positionMaxAge/time.Second) {
			continue
		}
		altitudeM, _ := rawFloat(state[7])
		velocityMps, _ := rawFloat(state[9])
		track, _ := rawFloat(state[10])
		verticalMps, _ := rawFloat(state[11])
		category := 0
		if len(state) > 17 {
			if value, ok := rawFloat(state[17]); ok {
				category = int(value)
			}
		}
		payload, err := json.Marshal(map[string]any{
			"hex":           strings.ToLower(hex),
			"callsign":      rawString(state[1]),
			"originCountry": rawString(state[2]),
			"kind":          kindForCategory(category),
			"squawk":        rawString(state[14]),
			"baroRateFpm":   verticalMps * 196.850394,
			"lat":           lat,
			"lon":           lon,
			"altFt":         altitudeM / 0.3048,
			"gsKt":          velocityMps * 1.943844,
			"track":         track,
			"onGround":      rawBool(state[8]),
			"validSeconds":  validSeconds,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID:   "opensky",
			SourceInstanceID: s.id,
			OriginalID:       fmt.Sprintf("opensky-%s-%s-%d", s.id, strings.ToLower(hex), now.Unix()),
			Domain:           "aviation",
			ObservedUTC:      now,
			Payload:          payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	backoff := time.Duration(0)
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("opensky sample failed", "source", s.id, "error", err)
		}
		if limited, ok := err.(errRateLimited); ok {
			if backoff == 0 {
				backoff = limited.retryAfter
			} else {
				backoff = min(backoff*2, backoffMax)
			}
			backoff = max(backoff, limited.retryAfter)
		} else if err == nil {
			backoff = 0
			s.logger.Info("opensky snapshot", "source", s.id, "aircraft", len(records))
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.interval + backoff):
		}
	}
}
