// Package kc2g polls the prop.kc2g.com station API (GIRO ionosonde network
// data, https://prop.kc2g.com, data courtesy of the Global Ionosphere Radio
// Observatory) and emits per-station ionospheric soundings: foF2, hmF2, foE,
// MUF(3000)F2 and TEC.
package kc2g

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	stationsURL = "https://prop.kc2g.com/api/stations.json"
	pollDefault = 10 * time.Minute
	pollFloor   = 5 * time.Minute
	// The feed keeps months-old readings for silent stations; anything older
	// than this is not a current picture of the ionosphere.
	maxReadingAge = 2 * time.Hour
)

// stationEntry mirrors the published API: numbers may be null, coordinates
// are strings, longitudes run 0-360, and md (the M(3000)F2 factor) is a
// string.
type stationEntry struct {
	Time    string   `json:"time"`
	FoF2    *float64 `json:"fof2"`
	HmF2    *float64 `json:"hmf2"`
	FoE     *float64 `json:"foe"`
	MufD    *float64 `json:"mufd"`
	TEC     *float64 `json:"tec"`
	CS      *float64 `json:"cs"`
	MD      string   `json:"md"`
	Station struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	} `json:"station"`
}

type sounding struct {
	StationID  string  `json:"stationId"`
	Name       string  `json:"name"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	FoF2Mhz    float64 `json:"foF2Mhz"`
	MufDMhz    float64 `json:"mufdMhz"`
	HmF2Km     any     `json:"hmF2Km"`
	FoEMhz     any     `json:"foEMhz"`
	M3000      any     `json:"m3000"`
	TEC        any     `json:"tec"`
	Confidence any     `json:"confidence"`
}

type Source struct {
	id       string
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger
	// lastEmitted dedupes readings per station across polls (the API returns
	// the same sounding until the next ionogram is processed).
	lastEmitted map[string]time.Time
	// fetch and now are swappable for tests.
	fetch func(ctx context.Context) ([]byte, error)
	now   func() time.Time
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "ionosonde.kc2g" {
		return nil, fmt.Errorf("unsupported KC2G source type %q", sourceConfig.Type)
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("KC2G poll interval below five minutes would hammer a community service (got %s)", interval)
	}
	if logger == nil {
		logger = slog.Default()
	}
	source := &Source{id: sourceConfig.ID, interval: interval, client: &http.Client{Timeout: 30 * time.Second}, logger: logger, lastEmitted: make(map[string]time.Time), now: time.Now}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "ionosonde.kc2g" }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		records, err := s.sample(ctx)
		if err != nil {
			s.logger.Warn("KC2G sample failed", "error", err)
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
		case <-ticker.C:
		}
	}
}

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, stationsURL, nil)
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
		return nil, fmt.Errorf("%s returned %s", stationsURL, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 8<<20))
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var entries []stationEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decode station list: %w", err)
	}
	now := s.now().UTC()
	records := make([]plugins.RawRecord, 0, len(entries))
	for _, entry := range entries {
		record, observed, ok := s.convert(entry, now)
		if !ok {
			continue
		}
		if last, seen := s.lastEmitted[entry.Station.Code]; seen && !observed.After(last) {
			continue
		}
		s.lastEmitted[entry.Station.Code] = observed
		records = append(records, record)
	}
	return records, nil
}

func (s *Source) convert(entry stationEntry, now time.Time) (plugins.RawRecord, time.Time, bool) {
	if entry.Station.Code == "" || entry.FoF2 == nil || entry.MufD == nil {
		return plugins.RawRecord{}, time.Time{}, false
	}
	observed, err := time.ParseInLocation("2006-01-02T15:04:05", entry.Time, time.UTC)
	if err != nil {
		s.logger.Warn("KC2G reading has an unparseable time", "station", entry.Station.Code, "time", entry.Time)
		return plugins.RawRecord{}, time.Time{}, false
	}
	if now.Sub(observed) > maxReadingAge {
		return plugins.RawRecord{}, time.Time{}, false
	}
	lat, latErr := strconv.ParseFloat(entry.Station.Latitude, 64)
	lon, lonErr := strconv.ParseFloat(entry.Station.Longitude, 64)
	if latErr != nil || lonErr != nil {
		s.logger.Warn("KC2G station has unparseable coordinates", "station", entry.Station.Code)
		return plugins.RawRecord{}, time.Time{}, false
	}
	if lon > 180 {
		lon -= 360
	}
	payload := sounding{
		StationID: entry.Station.Code,
		Name:      entry.Station.Name,
		Latitude:  lat,
		Longitude: lon,
		FoF2Mhz:   *entry.FoF2,
		MufDMhz:   *entry.MufD,
	}
	if entry.HmF2 != nil {
		payload.HmF2Km = *entry.HmF2
	}
	if entry.FoE != nil {
		payload.FoEMhz = *entry.FoE
	}
	if entry.TEC != nil {
		payload.TEC = *entry.TEC
	}
	if entry.CS != nil {
		payload.Confidence = *entry.CS
	}
	if m3000, err := strconv.ParseFloat(entry.MD, 64); err == nil {
		payload.M3000 = m3000
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return plugins.RawRecord{}, time.Time{}, false
	}
	return plugins.RawRecord{
		SourcePluginID:   "kc2g",
		SourceInstanceID: s.id,
		OriginalID:       fmt.Sprintf("kc2g-%s-%s-%d", s.id, entry.Station.Code, observed.Unix()),
		Domain:           "ionosphere",
		ObservedUTC:      observed,
		Payload:          encoded,
	}, observed, true
}
