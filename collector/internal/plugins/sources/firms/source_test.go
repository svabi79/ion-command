package firms

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

// testdata/viirs_snpp_sample.csv and testdata/modis_sample.csv are trimmed
// live captures from firms.modaps.eosdis.nasa.gov's no-key global 24h CSVs
// on 2026-08-27/28: seven rows near the Nevada/California example area of
// interest, plus low/nominal/high (VIIRS) and 0/32..100 (MODIS) confidence
// rows and an Aqua row, all outside that box.

func fixedNow(t *testing.T, value string) func() time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return func() time.Time { return parsed }
}

func viirsSource(t *testing.T) *Source {
	t.Helper()
	body, err := os.ReadFile("testdata/viirs_snpp_sample.csv")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "test", Type: "wildfire.firms"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context) ([]byte, error) { return body, nil }
	// All fixture rows fall on 2026-08-27; this keeps every one of them
	// inside the default 24h look-back window.
	source.now = fixedNow(t, "2026-08-27T23:00:00Z")
	return source
}

func modisSource(t *testing.T) *Source {
	t.Helper()
	body, err := os.ReadFile("testdata/modis_sample.csv")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "test", Type: "wildfire.firms", Satellite: "MODIS"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context) ([]byte, error) { return body, nil }
	source.now = fixedNow(t, "2026-08-27T23:00:00Z")
	return source
}

func TestSampleParsesViirsFixtureAndFiltersToArea(t *testing.T) {
	source := viirsSource(t)
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 13 rows in the fixture; 7 sit inside the default Nevada/California box,
	// 6 (Fiji, DR Congo/Kenya border, near the antimeridian) do not.
	if len(records) != 7 {
		t.Fatalf("expected 7 in-area detections, got %d", len(records))
	}
	var fields rawFields
	if err := json.Unmarshal(records[0].Payload, &fields); err != nil {
		t.Fatal(err)
	}
	if fields.Instrument != "VIIRS" || fields.SatelliteLabel != "Suomi NPP" {
		t.Fatalf("unexpected instrument/satellite: %+v", fields)
	}
	if fields.ConfidenceRaw != "nominal" {
		t.Fatalf("VIIRS confidence must pass through verbatim as a category word, got %q", fields.ConfidenceRaw)
	}
	if records[0].Domain != "wildfire" {
		t.Fatalf("unexpected domain %q", records[0].Domain)
	}
	wantObserved := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	if !records[0].ObservedUTC.Equal(wantObserved) {
		t.Fatalf("acq_date/acq_time parsed wrong: got %v want %v", records[0].ObservedUTC, wantObserved)
	}
	for _, record := range records {
		if !strings.Contains(record.OriginalID, "firms-test-N-2026-08-27-1000-") {
			continue // only the first cluster shares this acquisition time; fine.
		}
	}
}

func TestSampleParsesModisFixtureConfidenceAsPercent(t *testing.T) {
	source := modisSource(t)
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// 7 Terra rows sit inside the box; the two confidence=0 rows (Indonesia)
	// and two Aqua rows (Iran) do not.
	if len(records) != 7 {
		t.Fatalf("expected 7 in-area MODIS detections, got %d", len(records))
	}
	var fields rawFields
	if err := json.Unmarshal(records[0].Payload, &fields); err != nil {
		t.Fatal(err)
	}
	if fields.Instrument != "MODIS" || fields.SatelliteLabel != "Terra" {
		t.Fatalf("unexpected instrument/satellite: %+v", fields)
	}
	if fields.ConfidenceRaw != "100" {
		t.Fatalf("MODIS confidence must pass through verbatim as a numeric string, got %q", fields.ConfidenceRaw)
	}
}

func TestSampleDedupesAcrossPolls(t *testing.T) {
	source := viirsSource(t)
	first, err := source.sample(context.Background())
	if err != nil || len(first) != 7 {
		t.Fatalf("first sample unexpected: %d records, err=%v", len(first), err)
	}
	second, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("unchanged detections must not repeat, got %d", len(second))
	}
}

func TestSampleRespectsLookBackWindow(t *testing.T) {
	source := viirsSource(t)
	// Three days after the fixture's acquisition dates: every row is well
	// outside the default 24h look-back, even though it would still be
	// inside the area of interest.
	source.now = fixedNow(t, "2026-08-30T23:00:00Z")
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("stale detections outside the look-back window must not be emitted, got %d", len(records))
	}
}

func TestSampleExcludesRowsOutsideArea(t *testing.T) {
	source := viirsSource(t)
	records, _ := source.sample(context.Background())
	for _, record := range records {
		var fields rawFields
		if err := json.Unmarshal(record.Payload, &fields); err != nil {
			t.Fatal(err)
		}
		if fields.Longitude < source.west || fields.Longitude > source.east || fields.Latitude < source.south || fields.Latitude > source.north {
			t.Fatalf("detection %v outside the configured box leaked through", fields)
		}
	}
}

func TestSampleEnforcesRecordCap(t *testing.T) {
	source := viirsSource(t)
	source.maxRecords = 3 // fixture has 7 in-area rows; force truncation.
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("cap must be enforced, got %d records", len(records))
	}
	// A record dropped by the cap was never actually emitted, so it must not
	// be marked seen — otherwise it could never be emitted on a later poll
	// even though the collector never actually sent it.
	if len(source.seen) != 3 {
		t.Fatalf("only emitted records may be marked seen, seen has %d entries", len(source.seen))
	}
}

func TestSampleTruncationKeepsHighestFRP(t *testing.T) {
	source := viirsSource(t)
	source.maxRecords = 1
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly 1 record, got %d", len(records))
	}
	var fields rawFields
	if err := json.Unmarshal(records[0].Payload, &fields); err != nil {
		t.Fatal(err)
	}
	// Of the 7 in-area rows the fixture carries FRP values 8.71, 2.26, 6.15,
	// 1.63, 6.15, 6.15, 1.63 — 8.71 is the highest.
	if fields.FrpMw != 8.71 {
		t.Fatalf("truncation must keep the highest-FRP detection, got FRP %v", fields.FrpMw)
	}
}

func TestPurgeSeenBoundsTheDedupCache(t *testing.T) {
	source := viirsSource(t)
	source.seen["stale-entry"] = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, stillPresent := source.seen["stale-entry"]; stillPresent {
		t.Fatal("an entry older than the look-back window must be purged, not kept forever")
	}
}

func TestSeenCacheHardCapResets(t *testing.T) {
	source := viirsSource(t)
	source.maxSeen = 2
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The fixture emits 7 records; the hard cap of 2 must have fired and
	// reset the map rather than letting it grow past the bound.
	if len(source.seen) > source.maxSeen {
		t.Fatalf("seen cache exceeded its hard cap: %d > %d", len(source.seen), source.maxSeen)
	}
}

func TestSampleSkipsMalformedRowsWithoutFailingThePoll(t *testing.T) {
	// A hand-built CSV covering quirks not present in the live captures:
	// a short row, non-numeric coordinates, an out-of-range acq_time, and a
	// row with a blank FRP (real FIRMS rows occasionally omit it). One good
	// row anchors the fixture so a total failure would be obvious.
	quirky := "latitude,longitude,bright_ti4,scan,track,acq_date,acq_time,satellite,confidence,version,bright_ti5,frp,daynight\n" +
		"41.9,-117.4,300,0.4,0.4,2026-08-27,1000,N,nominal,2.0NRT,285,3.0,N\n" + // good
		"41.9,-117.4,300,0.4,0.4,2026-08-27,1000,N,nominal,2.0NRT,285\n" + // short row
		"notalat,-117.4,300,0.4,0.4,2026-08-27,1000,N,nominal,2.0NRT,285,3.0,N\n" + // bad latitude
		"41.9,-117.4,300,0.4,0.4,2026-08-27,2599,N,nominal,2.0NRT,285,3.0,N\n" + // impossible acq_time
		"41.9,-117.4,300,0.4,0.4,2026-08-27,1001,N,nominal,2.0NRT,285,,N\n" // blank FRP: tolerated, not invalid
	source, err := New(config.Source{ID: "test", Type: "wildfire.firms"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context) ([]byte, error) { return []byte(quirky), nil }
	source.now = fixedNow(t, "2026-08-27T23:00:00Z")
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatalf("a few malformed rows must not fail the whole poll: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected the 2 structurally valid rows (good + blank FRP), got %d", len(records))
	}
	var blankFRP rawFields
	if err := json.Unmarshal(records[1].Payload, &blankFRP); err != nil {
		t.Fatal(err)
	}
	if blankFRP.FrpMw != 0 {
		t.Fatalf("a blank FRP field should default to 0, not fail the row: got %v", blankFRP.FrpMw)
	}
}

func TestSampleRejectsUnrecognizedHeader(t *testing.T) {
	source, err := New(config.Source{ID: "test", Type: "wildfire.firms"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context) ([]byte, error) {
		return []byte("not,the,expected,columns\n1,2,3,4\n"), nil
	}
	if _, err := source.sample(context.Background()); err == nil {
		t.Fatal("a CSV missing the expected FIRMS columns must be rejected, not silently return zero rows")
	}
}

func TestDecodeSatellite(t *testing.T) {
	cases := []struct {
		code           string
		instrument     string
		satelliteLabel string
	}{
		{"T", "MODIS", "Terra"},
		{"A", "MODIS", "Aqua"},
		{"N", "VIIRS", "Suomi NPP"},
		{"N20", "VIIRS", "NOAA-20"},
		{"N21", "VIIRS", "NOAA-21"},
		{"X9", "", "X9"}, // unrecognized: passed through raw, not guessed.
	}
	for _, c := range cases {
		instrument, label := decodeSatellite(c.code)
		if instrument != c.instrument || label != c.satelliteLabel {
			t.Errorf("decodeSatellite(%q) = (%q, %q), want (%q, %q)", c.code, instrument, label, c.instrument, c.satelliteLabel)
		}
	}
}

func TestNewValidatesConfiguration(t *testing.T) {
	base := config.Source{ID: "test", Type: "wildfire.firms"}

	if _, err := New(config.Source{ID: "test", Type: "earthquake.usgs"}, nil); err == nil {
		t.Fatal("wrong type must be rejected")
	}
	tooFast := base
	tooFast.PollSeconds = 60
	if _, err := New(tooFast, nil); err == nil {
		t.Fatal("a poll interval below the 30-minute floor must be rejected")
	}
	badSatellite := base
	badSatellite.Satellite = "HUBBLE"
	if _, err := New(badSatellite, nil); err == nil {
		t.Fatal("an unsupported satellite must be rejected")
	}
	invertedBox := base
	invertedBox.BoxWest, invertedBox.BoxSouth, invertedBox.BoxEast, invertedBox.BoxNorth = 10, 10, 5, 20
	if _, err := New(invertedBox, nil); err == nil {
		t.Fatal("west >= east must be rejected")
	}
	badLookback := base
	badLookback.LookBackHours = 999
	if _, err := New(badLookback, nil); err == nil {
		t.Fatal("a look-back beyond the 7-day snapshot ceiling must be rejected")
	}

	source, err := New(base, nil)
	if err != nil {
		t.Fatalf("a minimal valid config must succeed: %v", err)
	}
	if source.satellite != defaultSatellite {
		t.Fatalf("expected default satellite %q, got %q", defaultSatellite, source.satellite)
	}
	if source.west != defaultWest || source.south != defaultSouth || source.east != defaultEast || source.north != defaultNorth {
		t.Fatalf("expected the default example box, got %v,%v,%v,%v", source.west, source.south, source.east, source.north)
	}
	if source.lookBack != defaultLookBackHours*time.Hour {
		t.Fatalf("expected default 24h look-back, got %v", source.lookBack)
	}
	if source.interval != pollDefault {
		t.Fatalf("expected default poll interval %v, got %v", pollDefault, source.interval)
	}
}

func TestRequestURLNoKeyPicksSatelliteAndWindow(t *testing.T) {
	source, err := New(config.Source{ID: "t", Type: "wildfire.firms", Satellite: "VIIRS_NOAA20", LookBackHours: 48}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := source.requestURL()
	want := "https://firms.modaps.eosdis.nasa.gov/data/active_fire/noaa-20-viirs-c2/csv/J1_VIIRS_C2_Global_48h.csv"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRequestURLWithMapKeyUsesAreaAPIAndRedacts(t *testing.T) {
	source, err := New(config.Source{ID: "t", Type: "wildfire.firms", MapKey: "SECRETKEY123", LookBackHours: 30}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := source.requestURL()
	want := "https://firms.modaps.eosdis.nasa.gov/api/area/csv/SECRETKEY123/VIIRS_SNPP_NRT/-124.5000,32.5000,-114.0000,42.0000/2"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if strings.Contains(source.redact(got), "SECRETKEY123") {
		t.Fatal("the MAP_KEY must never appear in redacted (loggable) text")
	}
}

func TestHandleResponseParsesRetryAfterOn429(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	_, err = handleResponse(response, "redacted-url")
	var limited errRateLimited
	if !errors.As(err, &limited) {
		t.Fatalf("expected errRateLimited, got %v", err)
	}
	if limited.retryAfter != 120*time.Second {
		t.Fatalf("expected Retry-After to parse as 120s, got %v", limited.retryAfter)
	}
}

func TestHandleResponseDefaultsRetryAfterWhenHeaderMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	_, err = handleResponse(response, "redacted-url")
	var limited errRateLimited
	if !errors.As(err, &limited) {
		t.Fatalf("expected errRateLimited, got %v", err)
	}
	if limited.retryAfter != 5*time.Minute {
		t.Fatalf("expected the 5-minute default when Retry-After is absent, got %v", limited.retryAfter)
	}
}

func TestHandleResponseReturnsBodyOn200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("latitude,longitude\n1,2\n"))
	}))
	defer server.Close()

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	body, err := handleResponse(response, "redacted-url")
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "latitude,longitude\n1,2\n" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestHandleResponseErrorNamesRedactedURLNotRealOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	_, err = handleResponse(response, "https://firms.modaps.eosdis.nasa.gov/api/area/csv/REDACTED/...")
	if err == nil || !strings.Contains(err.Error(), "REDACTED") || strings.Contains(err.Error(), server.URL) {
		t.Fatalf("error must report the caller-supplied (redacted) URL, not leak the real one: %v", err)
	}
}

func TestRequestURLDayRangeClampedToFive(t *testing.T) {
	source, err := New(config.Source{ID: "t", Type: "wildfire.firms", MapKey: "K", LookBackHours: 168}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(source.requestURL(), "/5") {
		t.Fatalf("day range must clamp to the Area API's documented maximum of 5, got %q", source.requestURL())
	}
}
