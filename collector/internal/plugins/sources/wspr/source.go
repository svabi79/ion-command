// Package wspr polls wspr.live (https://wspr.live), a free, credential-free
// ClickHouse mirror of the WSPRnet.org weak-signal beacon network, and turns
// recent reception reports into ham-radio raw records. WSPR stations
// transmit a low-power beacon on a strict schedule; any station that decodes
// one reports it automatically, which makes the network the most sensitive
// available picture of marginal HF propagation the collector has access to.
//
// Locators, not wspr.live's own precomputed lat/lon columns, are the source
// of truth for placement here (reusing pskreporter.MaidenheadToLatLon rather
// than reimplementing conversion) so a WSPR station's position matches the
// position PSKReporter or WSJT-X would compute for the exact same locator.
package wspr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/cty"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/pskreporter"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/wsjtx"
)

const (
	defaultBase = "https://db1.wspr.live/"
	pollDefault = 5 * time.Minute
	// wspr.live's own published guidance caps clients at 20 requests/minute;
	// this project asks not to poll a free community service aggressively
	// regardless, so the configurable floor sits far under that ceiling.
	pollFloor = 2 * time.Minute
	// Safety valve against a pathological burst. The steady-state global
	// rate observed during development was roughly 4,000-8,000 rows per two
	// minutes across all bands, well under this and far under wspr.live's
	// own 100,000-row response cap.
	rowLimit = 30000
	// Bounded dedupe window covering a few polls' worth of ids: the cursor
	// query uses ">=", not ">", to avoid losing same-second rows on a
	// boundary, so consecutive polls deliberately overlap by design.
	dedupeCapacity = 4 * rowLimit
	backoffInitial = 2 * time.Minute
	backoffMax     = 30 * time.Minute
	failureLimit   = 8
	// On startup there is no prior cursor; seed a short lookback instead of
	// either replaying the archive back to 2008 or starting completely idle.
	startupLookback = 2 * time.Minute
	httpTimeout     = 30 * time.Second
	maxResponseBody = 64 << 20
)

// wsprRow mirrors the minimal wspr.live wspr.rx column set actually used;
// selecting fewer columns follows wspr.live's own performance guidance.
type wsprRow struct {
	ID          uint64 `json:"id"`
	Timestamp   int64  `json:"ts"`
	TXCallsign  string `json:"tx_sign"`
	TXLocator   string `json:"tx_loc"`
	RXCallsign  string `json:"rx_sign"`
	RXLocator   string `json:"rx_loc"`
	FrequencyHz int64  `json:"frequency"`
	SNRDb       int    `json:"snr"`
}

type Source struct {
	id       string
	base     string
	interval time.Duration
	client   *http.Client
	logger   *slog.Logger

	cursorUnix int64
	seenIDs    map[uint64]struct{}
	seenOrder  []uint64

	// skippedUnplaceable counts rows with no usable locator on either end;
	// exported via Skipped() for tests and live-run verification.
	skippedUnplaceable uint64
	emitted            uint64

	// fetch and now are swappable for tests.
	fetch func(ctx context.Context, query string) ([]byte, error)
	now   func() time.Time
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "hamradio.wspr" {
		return nil, fmt.Errorf("unsupported wspr source type %q", sourceConfig.Type)
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("wspr poll interval below two minutes would poll a free community service too aggressively (got %s)", interval)
	}
	if logger == nil {
		logger = slog.Default()
	}
	base := defaultBase
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	source := &Source{
		id: sourceConfig.ID, base: base, interval: interval,
		client: &http.Client{Timeout: httpTimeout}, logger: logger,
		seenIDs: make(map[uint64]struct{}, dedupeCapacity), now: time.Now,
	}
	source.fetch = source.httpFetch
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "hamradio.wspr" }

// Skipped reports how many rows were dropped for lacking a usable locator,
// so a live run can report the real number rather than an assumed zero.
func (s *Source) Skipped() uint64 { return s.skippedUnplaceable }

// Emitted reports how many spots were successfully turned into raw records.
func (s *Source) Emitted() uint64 { return s.emitted }

// bandLabel classifies a frequency the same way wsjtx.BandFromFrequencyHz
// does (reused rather than reimplemented) but extends coverage to the two
// LF/MF bands WSPR is commonly used on that sit below that table's 160m
// floor. Anything else - including real but non-amateur activity observed
// live on wspr.live around 13.56 MHz (an ISM/experimental segment, not a
// ham band) - is deliberately left blank rather than guessed.
func bandLabel(frequencyHz int64) string {
	switch {
	case frequencyHz >= 135700 && frequencyHz <= 137800:
		return "2200m"
	case frequencyHz >= 472000 && frequencyHz <= 479000:
		return "630m"
	case frequencyHz >= 1240000000 && frequencyHz <= 1300000000:
		return "23cm"
	default:
		return wsjtx.BandFromFrequencyHz(frequencyHz)
	}
}

// regionNameFor resolves a callsign to a DXCC entity name via the country
// file (the same mechanism RBN and the DX cluster source use), purely as a
// bonus for the region breakdown panel - placement itself always comes from
// the reported Maidenhead locator, never from this lookup.
func regionNameFor(callsign string) string {
	if entity, ok := cty.Lookup(callsign); ok {
		return entity.Name
	}
	return ""
}

func buildQuery(sinceUnix int64) string {
	return fmt.Sprintf(
		"SELECT id, toUnixTimestamp(time) AS ts, tx_sign, tx_loc, rx_sign, rx_loc, frequency, snr "+
			"FROM wspr.rx WHERE time >= toDateTime(%d) ORDER BY time ASC LIMIT %d FORMAT JSONEachRow",
		sinceUnix, rowLimit)
}

func (s *Source) httpFetch(ctx context.Context, query string) ([]byte, error) {
	endpoint := strings.TrimRight(s.base, "/") + "/?query=" + url.QueryEscape(query)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBody))
	if response.StatusCode != http.StatusOK {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("%s returned %s: %s", s.base, response.Status, snippet)
	}
	if readErr != nil {
		return nil, readErr
	}
	return body, nil
}

// isDuplicate reports whether a row id has been seen recently, bounding the
// memory used for dedupe the same way the PSKReporter decoder does.
func (s *Source) isDuplicate(id uint64) bool {
	if _, seen := s.seenIDs[id]; seen {
		return true
	}
	if len(s.seenOrder) >= dedupeCapacity {
		oldest := s.seenOrder[0]
		s.seenOrder = s.seenOrder[1:]
		delete(s.seenIDs, oldest)
	}
	s.seenIDs[id] = struct{}{}
	s.seenOrder = append(s.seenOrder, id)
	return false
}

func (s *Source) convert(row wsprRow) (plugins.RawRecord, bool) {
	if row.TXCallsign == "" || row.RXCallsign == "" || row.FrequencyHz <= 0 {
		return plugins.RawRecord{}, false
	}
	txLat, txLon, err := pskreporter.MaidenheadToLatLon(row.TXLocator)
	if err != nil {
		return plugins.RawRecord{}, false
	}
	rxLat, rxLon, err := pskreporter.MaidenheadToLatLon(row.RXLocator)
	if err != nil {
		return plugins.RawRecord{}, false
	}
	payload, err := json.Marshal(map[string]any{
		"spotId":      fmt.Sprintf("wspr-%d", row.ID),
		"txCallsign":  row.TXCallsign,
		"rxCallsign":  row.RXCallsign,
		"txLongitude": txLon,
		"txLatitude":  txLat,
		"rxLongitude": rxLon,
		"rxLatitude":  rxLat,
		"frequencyHz": row.FrequencyHz,
		"band":        bandLabel(row.FrequencyHz),
		// wspr.live's "code" column distinguishes WSPR-2/WSPR-15/FST4W
		// variants, but no verified, authoritative mapping for its values
		// was found; rather than risk mislabelling a spot, every row from
		// this source is reported under the umbrella "WSPR".
		"mode":     "WSPR",
		"snrDb":    row.SNRDb,
		"txRegion": regionNameFor(row.TXCallsign),
		"rxRegion": regionNameFor(row.RXCallsign),
	})
	if err != nil {
		return plugins.RawRecord{}, false
	}
	return plugins.RawRecord{
		SourcePluginID:   "wspr",
		SourceInstanceID: s.id,
		OriginalID:       fmt.Sprintf("wspr-%s-%d", s.id, row.ID),
		Domain:           "hamradio",
		ObservedUTC:      time.Unix(row.Timestamp, 0).UTC(),
		Payload:          payload,
	}, true
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	if s.cursorUnix == 0 {
		s.cursorUnix = s.now().Add(-startupLookback).Unix()
	}
	body, err := s.fetch(ctx, buildQuery(s.cursorUnix))
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	var records []plugins.RawRecord
	maxTs := s.cursorUnix
	rows := 0
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var row wsprRow
		if err := json.Unmarshal(line, &row); err != nil {
			s.logger.Warn("WSPR row skipped: malformed JSON", "error", err)
			continue
		}
		rows++
		if row.Timestamp > maxTs {
			maxTs = row.Timestamp
		}
		if s.isDuplicate(row.ID) {
			continue
		}
		record, ok := s.convert(row)
		if !ok {
			s.skippedUnplaceable++
			continue
		}
		s.emitted++
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return records, fmt.Errorf("read WSPR response: %w", err)
	}
	if rows >= rowLimit {
		s.logger.Warn("WSPR response hit the row limit; some spots in this window may have been truncated", "limit", rowLimit)
	}
	// Advance even past duplicate/unplaceable rows so a run of unusable rows
	// at the same timestamp cannot stall the cursor.
	s.cursorUnix = maxTs
	return records, nil
}

// Start polls on the configured interval with capped exponential backoff on
// failure and a hard stop after repeated consecutive failures, matching the
// discipline the other HTTP-poll sources (CelesTrak, OpenSky) apply against
// rate-sensitive public APIs. Unlike CelesTrak - which keeps a separate
// always-running propagation loop independent of its fetch - polling is
// WSPR's only job, so giving up is expressed by actually returning an error
// (visible as source state "failed" via /api/status) rather than silently
// looping forever having stopped trying.
func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	backoff := time.Duration(0)
	consecutiveFailures := 0
	for {
		records, err := s.sample(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			consecutiveFailures++
			if backoff == 0 {
				backoff = backoffInitial
			} else if backoff < backoffMax {
				backoff *= 2
			}
			if backoff > backoffMax {
				backoff = backoffMax
			}
			s.logger.Warn("WSPR sample failed; backing off", "source", s.id, "error", err,
				"consecutiveFailures", consecutiveFailures, "retryIn", (s.interval + backoff).String())
			if consecutiveFailures >= failureLimit {
				return fmt.Errorf("WSPR source failed %d times in a row, stopping: %w", consecutiveFailures, err)
			}
		case err == nil:
			if consecutiveFailures > 0 {
				s.logger.Info("WSPR recovered", "afterFailures", consecutiveFailures)
			}
			consecutiveFailures = 0
			backoff = 0
			s.logger.Info("WSPR sample ok", "source", s.id, "spots", len(records), "skippedUnplaceable", s.skippedUnplaceable)
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
