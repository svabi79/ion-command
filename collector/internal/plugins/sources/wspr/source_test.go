package wspr

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func testSource(t *testing.T) *Source {
	t.Helper()
	source, err := New(config.Source{ID: "test", Type: "hamradio.wspr"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	return source
}

// testdata/sample_live.jsonl is a live capture from https://db1.wspr.live in
// exactly the shape buildQuery() requests, taken 2026-08-28 ~16:04 UTC.
func liveSource(t *testing.T) *Source {
	t.Helper()
	body, err := os.ReadFile("testdata/sample_live.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	source := testSource(t)
	source.fetch = func(_ context.Context, _ string) ([]byte, error) { return body, nil }
	return source
}

type decodedSpot struct {
	SpotID      string  `json:"spotId"`
	TXCallsign  string  `json:"txCallsign"`
	RXCallsign  string  `json:"rxCallsign"`
	TXLongitude float64 `json:"txLongitude"`
	TXLatitude  float64 `json:"txLatitude"`
	RXLongitude float64 `json:"rxLongitude"`
	RXLatitude  float64 `json:"rxLatitude"`
	FrequencyHz int64   `json:"frequencyHz"`
	Band        string  `json:"band"`
	Mode        string  `json:"mode"`
	SNRDb       *int    `json:"snrDb"`
	TXRegion    string  `json:"txRegion"`
	RXRegion    string  `json:"rxRegion"`
}

func TestSampleParsesLiveFeed(t *testing.T) {
	source := liveSource(t)
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 6 {
		t.Fatalf("expected 6 spots from the live fixture, got %d", len(records))
	}
	if source.Skipped() != 0 {
		t.Fatalf("every fixture row has usable locators, expected 0 skipped, got %d", source.Skipped())
	}
	if source.Emitted() != 6 {
		t.Fatalf("expected 6 emitted, got %d", source.Emitted())
	}
	var first decodedSpot
	if err := json.Unmarshal(records[0].Payload, &first); err != nil {
		t.Fatal(err)
	}
	if first.TXCallsign != "G4HSB" || first.RXCallsign != "DL7VXX" {
		t.Fatalf("unexpected first spot: %+v", first)
	}
	if first.Band != "20m" || first.Mode != "WSPR" {
		t.Fatalf("unexpected band/mode: %+v", first)
	}
	if first.SNRDb == nil || *first.SNRDb != -11 {
		t.Fatalf("unexpected snr: %+v", first.SNRDb)
	}
	if records[0].Domain != "hamradio" {
		t.Fatalf("unexpected domain %q", records[0].Domain)
	}
	// Locator-derived coordinates, not wspr.live's own precomputed lat/lon.
	// IO94ll -> ~54.48N, 1.04W (northern England); verified against
	// pskreporter.MaidenheadToLatLon directly before asserting this range.
	if first.TXLatitude < 53.0 || first.TXLatitude > 56.0 || first.TXLongitude < -3.0 || first.TXLongitude > 1.0 {
		t.Fatalf("IO94ll should place in northern England: %+v", first)
	}
}

// TestZeroSNRIsNotAbsent guards against a regression where 0 is mistaken for
// "no report": WSPR's snr column is never null, so 0 dB is a real report,
// unlike the DX cluster's genuinely optional signal report.
func TestZeroSNRIsNotAbsent(t *testing.T) {
	source := liveSource(t)
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		var spot decodedSpot
		if err := json.Unmarshal(record.Payload, &spot); err != nil {
			t.Fatal(err)
		}
		if spot.TXCallsign == "UG3B" {
			found = true
			if spot.SNRDb == nil {
				t.Fatal("0 dB SNR must not be reported as absent")
			}
			if *spot.SNRDb != 0 {
				t.Fatalf("expected 0 dB, got %d", *spot.SNRDb)
			}
		}
	}
	if !found {
		t.Fatal("fixture row for UG3B not found")
	}
}

// TestSlashCallsignsSurviveDecoding covers WSPR's SWL (receive-only) and
// portable-suffix callsigns, e.g. "DK7FC/RX" and "HB9VQQ/RE", captured live.
func TestSlashCallsignsSurviveDecoding(t *testing.T) {
	source := testSource(t)
	row := wsprRow{ID: 1, Timestamp: 1787932800, TXCallsign: "OE9RMV", TXLocator: "JN47ul", RXCallsign: "DK7FC/RX", RXLocator: "JN49ik", FrequencyHz: 137483, SNRDb: -17}
	record, ok := source.convert(row)
	if !ok {
		t.Fatal("row with a slashed SWL callsign must convert")
	}
	var spot decodedSpot
	if err := json.Unmarshal(record.Payload, &spot); err != nil {
		t.Fatal(err)
	}
	if spot.RXCallsign != "DK7FC/RX" {
		t.Fatalf("callsign mangled: %q", spot.RXCallsign)
	}
	if spot.Band != "2200m" {
		t.Fatalf("137.483 kHz must classify as 2200m, got %q", spot.Band)
	}
}

func TestBandLabelExtendsWsjtxTable(t *testing.T) {
	cases := []struct {
		name        string
		frequencyHz int64
		want        string
	}{
		{"2200m, real capture 137483 Hz", 137483, "2200m"},
		{"630m, real capture 475732 Hz", 475732, "630m"},
		{"20m, delegated to wsjtx table", 14097199, "20m"},
		{"23cm, real capture 1296501582 Hz", 1296501582, "23cm"},
		{"non-ham ISM/experimental segment, real capture 13555393 Hz", 13555393, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bandLabel(c.frequencyHz); got != c.want {
				t.Fatalf("bandLabel(%d) = %q, want %q", c.frequencyHz, got, c.want)
			}
		})
	}
}

// TestSampleSkipsUnplaceableRows covers the "do not invent coordinates"
// discipline. No genuinely locator-less row was observed live in ~125,000
// rows sampled during development (wsprnet.org appears to reject them at
// ingest), so this fixture is a synthetic worst case rather than a live
// capture - clearly marked as such, unlike every other fixture in this file.
func TestSampleSkipsUnplaceableRows(t *testing.T) {
	source := testSource(t)
	synthetic := `{"id":1,"ts":1787932800,"tx_sign":"HB9ABC","tx_loc":"","rx_sign":"K1ABC","rx_loc":"FN31pr","frequency":14097100,"snr":-10}
{"id":2,"ts":1787932800,"tx_sign":"HB9ABC","tx_loc":"JN47ka","rx_sign":"K1ABC","rx_loc":"FN31pr","frequency":14097100,"snr":-10}
`
	source.fetch = func(_ context.Context, _ string) ([]byte, error) { return []byte(synthetic), nil }
	records, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected exactly the one placeable row, got %d", len(records))
	}
	if source.Skipped() != 1 {
		t.Fatalf("expected 1 skipped for a missing locator, got %d", source.Skipped())
	}
}

// TestSampleDedupesOverlappingPolls proves the cursor's ">=" boundary
// overlap does not double-emit: the same id returned by two consecutive
// polls (as wspr.live will when a poll re-requests from the last-seen
// second) must only be emitted once.
func TestSampleDedupesOverlappingPolls(t *testing.T) {
	source := testSource(t)
	batch := `{"id":42,"ts":1787932800,"tx_sign":"HB9ABC","tx_loc":"JN47ka","rx_sign":"K1ABC","rx_loc":"FN31pr","frequency":14097100,"snr":-10}
`
	source.fetch = func(_ context.Context, _ string) ([]byte, error) { return []byte(batch), nil }
	first, err := source.sample(context.Background())
	if err != nil || len(first) != 1 {
		t.Fatalf("first poll: %v (%d records)", err, len(first))
	}
	second, err := source.sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("redelivered id must be deduped, got %d records", len(second))
	}
}

// TestCursorSeedsRecentLookbackOnly proves startup does not attempt to
// replay wspr.live's full history back to 2008.
func TestCursorSeedsRecentLookbackOnly(t *testing.T) {
	source := testSource(t)
	fixedNow := time.Date(2026, 8, 28, 16, 0, 0, 0, time.UTC)
	source.now = func() time.Time { return fixedNow }
	var capturedQuery string
	source.fetch = func(_ context.Context, query string) ([]byte, error) {
		capturedQuery = query
		return nil, nil
	}
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantSince := fixedNow.Add(-startupLookback).Unix()
	wantFragment := "toDateTime(" + strconv.FormatInt(wantSince, 10) + ")"
	if !strings.Contains(capturedQuery, wantFragment) {
		t.Fatalf("expected query to seed from %s, got %q", wantFragment, capturedQuery)
	}
}

func TestPollIntervalFloorEnforced(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "hamradio.wspr", PollSeconds: 30}, slog.Default()); err == nil {
		t.Fatal("poll interval under the floor must be rejected")
	}
}

func TestWrongTypeRejected(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "hamradio.rbn"}, slog.Default()); err == nil {
		t.Fatal("wrong source type must be rejected")
	}
}
