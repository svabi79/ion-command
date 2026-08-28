// Package firms polls NASA FIRMS (Fire Information for Resource Management
// System) active-fire products and emits one raw record per VIIRS/MODIS
// thermal-anomaly detection inside a configured area of interest and
// look-back window.
//
// FIRMS publishes the same CSV shape through two access paths:
//
//   - Global per-satellite snapshots (24h/48h/7d), regenerated roughly
//     hourly, reachable with no credentials at all:
//     https://firms.modaps.eosdis.nasa.gov/data/active_fire/. This is the
//     default path: it works out of the box, at the cost of downloading a
//     whole-world file and filtering locally.
//   - The MAP_KEY-scoped Area API, which filters server-side to a bounding
//     box and is far lighter on the wire. It needs a free key from
//     https://firms.modaps.eosdis.nasa.gov/api/map_key/. Setting "mapKey"
//     on a source instance switches to this path.
//
// See docs/DATA-SOURCES.md for NASA's attribution and terms.
package firms

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	globalBase = "https://firms.modaps.eosdis.nasa.gov/data/active_fire"
	areaBase   = "https://firms.modaps.eosdis.nasa.gov/api/area/csv"

	// FIRMS regenerates the NRT CSV/SHP/KML snapshots roughly once an hour
	// (documented cadence); the Area API shares the same underlying data.
	// Polling faster than every thirty minutes would hammer NASA for data
	// that has not changed, key or no key.
	pollDefault = 3 * time.Hour
	pollFloor   = 30 * time.Minute

	defaultLookBackHours = 24.0
	maxLookBackHours     = 168.0 // 7 days: the longest no-key snapshot window.

	// The default area of interest ships as one illustrative example — the
	// US West, a reliably active wildfire region — same idea as the
	// adsb-region-example source shipping one example circle. Replace it.
	defaultWest  = -124.5
	defaultSouth = 32.5
	defaultEast  = -114.0
	defaultNorth = 42.0

	defaultSatellite = "VIIRS_SNPP"

	// Hard safety valve: whatever the configured box, never emit more than
	// this many detections from one poll. Excess is dropped least-energetic
	// first (ascending FRP) and logged, not silently swallowed.
	maxRecordsPerSample = 5000

	// Read cap for the fetched body. The global VIIRS CSV is ~16 MB today;
	// this leaves headroom for a very active fire season without being
	// unbounded.
	maxBodyBytes = 64 << 20

	// Bound on the cross-poll dedup cache. The default area keeps a few
	// hundred entries; this is a safety valve for a much wider configured
	// box, not the normal operating size.
	maxSeenEntries = 50000
)

// satelliteProfile carries the two URL vocabularies FIRMS uses for the same
// instrument: the no-key global snapshot path and the MAP_KEY Area API's
// SOURCE parameter.
type satelliteProfile struct {
	globalFeed   string
	globalPrefix string
	areaSource   string
}

var satelliteProfiles = map[string]satelliteProfile{
	"VIIRS_SNPP":   {globalFeed: "suomi-npp-viirs-c2", globalPrefix: "SUOMI_VIIRS_C2", areaSource: "VIIRS_SNPP_NRT"},
	"VIIRS_NOAA20": {globalFeed: "noaa-20-viirs-c2", globalPrefix: "J1_VIIRS_C2", areaSource: "VIIRS_NOAA20_NRT"},
	"VIIRS_NOAA21": {globalFeed: "noaa-21-viirs-c2", globalPrefix: "J2_VIIRS_C2", areaSource: "VIIRS_NOAA21_NRT"},
	"MODIS":        {globalFeed: "modis-c6.1", globalPrefix: "MODIS_C6_1", areaSource: "MODIS_NRT"},
}

// SatelliteNames lists the values the "satellite" config field accepts, in a
// stable order, for error messages and documentation.
func SatelliteNames() []string {
	return []string{"VIIRS_SNPP", "VIIRS_NOAA20", "VIIRS_NOAA21", "MODIS"}
}

// errRateLimited carries the server-requested pause from a 429 response so
// Start can extend the next wait without a persistent backoff state machine
// — at hours-scale polling, a single extra pause is sufficient.
type errRateLimited struct{ retryAfter time.Duration }

func (e errRateLimited) Error() string {
	return fmt.Sprintf("rate limited by FIRMS (retry after %s)", e.retryAfter)
}

// rawFields is the payload a raw record carries to the wildfire domain. It
// is the source's factual read of the row: which sensor and satellite
// produced it (decoded from FIRMS' own short codes, not interpreted), and
// the confidence value verbatim in whatever encoding the row used. Anything
// resembling presentation or an assessment of what the data proves (a
// display string, a normalized confidence score, a validity window) is the
// domain's job, not the source's.
type rawFields struct {
	DetectionID    string  `json:"detectionId"`
	Longitude      float64 `json:"longitude"`
	Latitude       float64 `json:"latitude"`
	Instrument     string  `json:"instrument"`     // "MODIS" | "VIIRS" | "" if the satellite code is unrecognized
	SatelliteCode  string  `json:"satelliteCode"`  // raw FIRMS code, e.g. "T", "N20"
	SatelliteLabel string  `json:"satelliteLabel"` // decoded, e.g. "Terra", "NOAA-20"
	ConfidenceRaw  string  `json:"confidenceRaw"`  // verbatim: "0".."100" (MODIS) or low/nominal/high (VIIRS)
	BrightnessK    float64 `json:"brightnessK"`    // brightness (MODIS) / bright_ti4 (VIIRS), kelvin
	Brightness2K   float64 `json:"brightness2K"`   // bright_t31 (MODIS) / bright_ti5 (VIIRS), kelvin
	FrpMw          float64 `json:"frpMw"`          // fire radiative power, megawatts
	DayNight       string  `json:"dayNight"`       // "D" | "N"
}

type csvColumns struct {
	latitude, longitude      int
	brightness1, brightness2 int
	acqDate, acqTime         int
	satellite, confidence    int
	frp, dayNight            int
}

func (c csvColumns) maxIndex() int {
	max := c.latitude
	for _, v := range []int{c.longitude, c.brightness1, c.brightness2, c.acqDate, c.acqTime, c.satellite, c.confidence, c.frp, c.dayNight} {
		if v > max {
			max = v
		}
	}
	return max
}

// resolveColumns builds a name->index map from the CSV header instead of
// trusting column position, so MODIS's "brightness"/"bright_t31" and VIIRS's
// "bright_ti4"/"bright_ti5" both resolve through the same reader.
func resolveColumns(header []string) (csvColumns, error) {
	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}
	find := func(names ...string) (int, bool) {
		for _, name := range names {
			if i, ok := index[name]; ok {
				return i, true
			}
		}
		return 0, false
	}
	var columns csvColumns
	for _, field := range []struct {
		dest  *int
		names []string
	}{
		{&columns.latitude, []string{"latitude"}},
		{&columns.longitude, []string{"longitude"}},
		{&columns.brightness1, []string{"brightness", "bright_ti4"}},
		{&columns.brightness2, []string{"bright_t31", "bright_ti5"}},
		{&columns.acqDate, []string{"acq_date"}},
		{&columns.acqTime, []string{"acq_time"}},
		{&columns.satellite, []string{"satellite"}},
		{&columns.confidence, []string{"confidence"}},
		{&columns.frp, []string{"frp"}},
		{&columns.dayNight, []string{"daynight"}},
	} {
		i, ok := find(field.names...)
		if !ok {
			return csvColumns{}, fmt.Errorf("FIRMS CSV missing expected column (one of %v)", field.names)
		}
		*field.dest = i
	}
	return columns, nil
}

// decodeSatellite turns FIRMS' short satellite code into an instrument
// family and a human-readable label. This is a direct decode of the wire
// code (there are exactly five values FIRMS documents), not an assessment of
// the data, so it lives with the source rather than the domain — the same
// reasoning the adsb source uses for its emitter-category decode.
func decodeSatellite(code string) (instrument, label string) {
	switch code {
	case "T":
		return "MODIS", "Terra"
	case "A":
		return "MODIS", "Aqua"
	case "N":
		return "VIIRS", "Suomi NPP"
	case "N20":
		return "VIIRS", "NOAA-20"
	case "N21":
		return "VIIRS", "NOAA-21"
	default:
		return "", code
	}
}

type Source struct {
	id                       string
	satellite                string
	profile                  satelliteProfile
	west, south, east, north float64
	lookBack                 time.Duration
	mapKey                   string
	interval                 time.Duration
	client                   *http.Client
	logger                   *slog.Logger

	// seen dedupes detections across overlapping poll windows; the value is
	// the detection's acquisition time so entries can be purged once they
	// age out of the look-back window rather than growing forever.
	seen map[string]time.Time

	// maxRecords and maxSeen default to maxRecordsPerSample/maxSeenEntries;
	// they are fields rather than bare constants so a test can shrink them
	// and prove the bounding logic actually fires on a small fixture.
	maxRecords int
	maxSeen    int

	// fetch and now are swappable for tests.
	fetch func(ctx context.Context) ([]byte, error)
	now   func() time.Time
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "wildfire.firms" {
		return nil, fmt.Errorf("unsupported FIRMS source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("FIRMS poll interval below thirty minutes would hammer NASA for data that only changes hourly (got %s)", interval)
	}

	satellite := sourceConfig.Satellite
	if satellite == "" {
		satellite = defaultSatellite
	}
	profile, ok := satelliteProfiles[satellite]
	if !ok {
		return nil, fmt.Errorf("unsupported FIRMS satellite %q (want one of %v)", satellite, SatelliteNames())
	}

	west, south, east, north := defaultWest, defaultSouth, defaultEast, defaultNorth
	if sourceConfig.BoxWest != 0 || sourceConfig.BoxSouth != 0 || sourceConfig.BoxEast != 0 || sourceConfig.BoxNorth != 0 {
		west, south, east, north = sourceConfig.BoxWest, sourceConfig.BoxSouth, sourceConfig.BoxEast, sourceConfig.BoxNorth
	}
	if err := validateBox(west, south, east, north); err != nil {
		return nil, err
	}

	lookBackHours := sourceConfig.LookBackHours
	if lookBackHours == 0 {
		lookBackHours = defaultLookBackHours
	}
	if lookBackHours < 0 || lookBackHours > maxLookBackHours {
		return nil, fmt.Errorf("FIRMS lookBackHours must be between 0 (meaning default 24h) and %g, got %g", maxLookBackHours, lookBackHours)
	}

	source := &Source{
		id: sourceConfig.ID, satellite: satellite, profile: profile,
		west: west, south: south, east: east, north: north,
		lookBack:   time.Duration(lookBackHours * float64(time.Hour)),
		mapKey:     sourceConfig.MapKey,
		interval:   interval,
		client:     &http.Client{Timeout: 90 * time.Second},
		logger:     logger,
		seen:       make(map[string]time.Time),
		maxRecords: maxRecordsPerSample,
		maxSeen:    maxSeenEntries,
		now:        time.Now,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func validateBox(west, south, east, north float64) error {
	if west < -180 || west > 180 || east < -180 || east > 180 {
		return fmt.Errorf("FIRMS box longitude out of WGS84 range: west=%v east=%v", west, east)
	}
	if south < -90 || south > 90 || north < -90 || north > 90 {
		return fmt.Errorf("FIRMS box latitude out of WGS84 range: south=%v north=%v", south, north)
	}
	if west >= east {
		return fmt.Errorf("FIRMS box west must be less than east (no antimeridian wraparound support): west=%v east=%v", west, east)
	}
	if south >= north {
		return fmt.Errorf("FIRMS box south must be less than north: south=%v north=%v", south, north)
	}
	return nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "wildfire.firms" }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	extra := time.Duration(0)
	for {
		records, err := s.sample(ctx)
		var limited errRateLimited
		if errors.As(err, &limited) {
			extra = limited.retryAfter
			s.logger.Warn("FIRMS sample rate limited", "source", s.id, "retryAfter", extra)
		} else {
			extra = 0
			if err != nil {
				s.logger.Warn("FIRMS sample failed", "source", s.id, "error", err)
			}
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
		case <-time.After(s.interval + extra):
		}
	}
}

func windowSuffix(lookBack time.Duration) string {
	switch {
	case lookBack <= 24*time.Hour:
		return "24h"
	case lookBack <= 48*time.Hour:
		return "48h"
	default:
		return "7d"
	}
}

func (s *Source) requestURL() string {
	if s.mapKey != "" {
		dayRange := int(math.Ceil(s.lookBack.Hours() / 24))
		if dayRange < 1 {
			dayRange = 1
		}
		if dayRange > 5 {
			dayRange = 5 // FIRMS Area API's documented per-request maximum.
		}
		area := fmt.Sprintf("%.4f,%.4f,%.4f,%.4f", s.west, s.south, s.east, s.north)
		return fmt.Sprintf("%s/%s/%s/%s/%d", areaBase, s.mapKey, s.profile.areaSource, area, dayRange)
	}
	return fmt.Sprintf("%s/%s/csv/%s_Global_%s.csv", globalBase, s.profile.globalFeed, s.profile.globalPrefix, windowSuffix(s.lookBack))
}

// redact hides the MAP_KEY path segment from anything that might end up in
// logs or returned errors.
func (s *Source) redact(text string) string {
	if s.mapKey == "" {
		return text
	}
	return strings.ReplaceAll(text, s.mapKey, "REDACTED")
}

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	requestURL := s.requestURL()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.redact(requestURL), err)
	}
	defer response.Body.Close()
	return handleResponse(response, s.redact(requestURL))
}

// handleResponse decides what httpFetch returns for a given HTTP response:
// the body on 200, a typed rate-limit error (honouring Retry-After) on 429,
// or a plain error naming the (redacted) URL for anything else. Split out
// from httpFetch so this decision can be unit-tested against hand-built
// responses without a real round trip or a live MAP_KEY.
func handleResponse(response *http.Response, redactedURL string) ([]byte, error) {
	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5 * time.Minute
		if header := response.Header.Get("Retry-After"); header != "" {
			if seconds, parseErr := strconv.Atoi(strings.TrimSpace(header)); parseErr == nil && seconds > 0 {
				retryAfter = time.Duration(seconds) * time.Second
			}
		}
		return nil, errRateLimited{retryAfter: retryAfter}
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", redactedURL, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, maxBodyBytes))
}

type candidate struct {
	record plugins.RawRecord
	frpMw  float64
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(bytes.NewReader(body))
	reader.FieldsPerRecord = -1 // tolerate ragged rows; short ones are rejected explicitly below.
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read FIRMS CSV header: %w", err)
	}
	columns, err := resolveColumns(header)
	if err != nil {
		return nil, err
	}

	now := s.now().UTC()
	cutoff := now.Add(-s.lookBack)
	s.purgeSeen(cutoff)

	var fetched, inAreaAndWindow, duplicates, invalid int
	candidates := make([]candidate, 0, 256)
	polledIDs := make(map[string]bool)
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			invalid++
			continue
		}
		fetched++
		if len(row) <= columns.maxIndex() {
			invalid++
			continue
		}
		fields, observed, ok := parseRow(row, columns)
		if !ok {
			invalid++
			continue
		}
		if observed.Before(cutoff) || observed.After(now.Add(time.Hour)) {
			// Outside the requested look-back window, or implausibly in the
			// future (clock skew or a malformed date) — drop either way.
			continue
		}
		if fields.Latitude < s.south || fields.Latitude > s.north || fields.Longitude < s.west || fields.Longitude > s.east {
			continue
		}
		inAreaAndWindow++
		if _, already := s.seen[fields.DetectionID]; already || polledIDs[fields.DetectionID] {
			duplicates++
			continue
		}
		polledIDs[fields.DetectionID] = true
		payload, err := json.Marshal(fields)
		if err != nil {
			invalid++
			continue
		}
		candidates = append(candidates, candidate{
			frpMw: fields.FrpMw,
			record: plugins.RawRecord{
				SourcePluginID:   "firms",
				SourceInstanceID: s.id,
				OriginalID:       fmt.Sprintf("firms-%s-%s", s.id, fields.DetectionID),
				Domain:           "wildfire",
				ObservedUTC:      observed,
				Payload:          payload,
			},
		})
	}

	truncated := 0
	if len(candidates) > s.maxRecords {
		truncated = len(candidates) - s.maxRecords
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].frpMw > candidates[j].frpMw })
		candidates = candidates[:s.maxRecords]
	}

	records := make([]plugins.RawRecord, 0, len(candidates))
	for _, c := range candidates {
		// Only mark emitted detections as seen. One that got cut by the cap
		// above was never actually published, so it must stay eligible for a
		// future poll rather than being suppressed as a "duplicate" of a
		// message the collector never sent.
		s.seen[extractDetectionID(c.record)] = c.record.ObservedUTC
		records = append(records, c.record)
	}

	if len(s.seen) > s.maxSeen {
		s.logger.Warn("FIRMS dedup cache exceeded its bound after purge; resetting", "source", s.id, "size", len(s.seen))
		s.seen = make(map[string]time.Time)
	}

	s.logger.Info("FIRMS sample", "source", s.id, "satellite", s.satellite,
		"fetchedRows", fetched, "inAreaAndWindow", inAreaAndWindow, "emitted", len(records),
		"duplicatesSuppressed", duplicates, "invalidRows", invalid, "truncatedByCap", truncated)
	return records, nil
}

// extractDetectionID recovers the dedup key from a record already built for
// emission, so the seen-map is only updated once, after truncation.
func extractDetectionID(record plugins.RawRecord) string {
	var fields rawFields
	_ = json.Unmarshal(record.Payload, &fields)
	return fields.DetectionID
}

func (s *Source) purgeSeen(cutoff time.Time) {
	for key, observed := range s.seen {
		if observed.Before(cutoff) {
			delete(s.seen, key)
		}
	}
}

// parseRow decodes one CSV data row into rawFields plus its UTC acquisition
// time. FIRMS' acq_time is a zero-padded 24-hour "HHMM" with no separator;
// it is parsed by hand rather than through time.Parse's reference layout to
// keep the failure mode explicit for malformed rows (an occasional real-file
// quirk) instead of relying on stdlib layout-matching edge cases.
func parseRow(row []string, columns csvColumns) (rawFields, time.Time, bool) {
	latStr := strings.TrimSpace(row[columns.latitude])
	lonStr := strings.TrimSpace(row[columns.longitude])
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		return rawFields{}, time.Time{}, false
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		return rawFields{}, time.Time{}, false
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return rawFields{}, time.Time{}, false
	}

	dateStr := strings.TrimSpace(row[columns.acqDate])
	datePart, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return rawFields{}, time.Time{}, false
	}
	timeStr := strings.TrimSpace(row[columns.acqTime])
	if len(timeStr) == 0 || len(timeStr) > 4 {
		return rawFields{}, time.Time{}, false
	}
	for len(timeStr) < 4 {
		timeStr = "0" + timeStr
	}
	hour, err1 := strconv.Atoi(timeStr[:2])
	minute, err2 := strconv.Atoi(timeStr[2:])
	if err1 != nil || err2 != nil || hour > 23 || minute > 59 {
		return rawFields{}, time.Time{}, false
	}
	observed := time.Date(datePart.Year(), datePart.Month(), datePart.Day(), hour, minute, 0, 0, time.UTC)

	satelliteCode := strings.TrimSpace(row[columns.satellite])
	instrument, satelliteLabel := decodeSatellite(satelliteCode)
	// brightness/frp default to zero on parse failure rather than rejecting
	// the row: a missing radiometric value does not invalidate a detection
	// whose position and time are otherwise good.
	brightness1, _ := strconv.ParseFloat(strings.TrimSpace(row[columns.brightness1]), 64)
	brightness2, _ := strconv.ParseFloat(strings.TrimSpace(row[columns.brightness2]), 64)
	frp, _ := strconv.ParseFloat(strings.TrimSpace(row[columns.frp]), 64)

	fields := rawFields{
		DetectionID:    fmt.Sprintf("%s-%s-%s-%s-%s", satelliteCode, dateStr, timeStr, latStr, lonStr),
		Longitude:      lon,
		Latitude:       lat,
		Instrument:     instrument,
		SatelliteCode:  satelliteCode,
		SatelliteLabel: satelliteLabel,
		ConfidenceRaw:  strings.TrimSpace(row[columns.confidence]),
		BrightnessK:    brightness1,
		Brightness2K:   brightness2,
		FrpMw:          frp,
		DayNight:       strings.TrimSpace(row[columns.dayNight]),
	}
	return fields, observed, true
}
