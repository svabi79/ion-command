package aprs

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

// TestFixtureLiveCapture replays real APRS-IS packet lines captured live on
// 2026-08-28 (read-only login "NOCALL", filter "r/50.0/8.0/300 t/poi"
// against rotate.aprs2.net:14580) through the full Normalize pipeline.
//
// The corpus is a diversity-capped sample (up to 40 lines per observed
// data-type indicator, all 11 Item lines and all 20 apostrophe-Mic-E lines
// included since those were rare) drawn from a larger 2-minute capture, not
// a representative traffic mix - it deliberately over-samples rare and
// malformed-looking packet shapes for coverage, which is why its decode
// rate is lower than the roughly 88% observed on the raw, unfiltered
// capture (8544 lines in, 7543 envelopes out; see the collector agent's
// final report for the exact session).
//
// This is primarily a robustness/regression check: real traffic constantly
// contains malformed packets from buggy trackers (truncated fields, decimal
// degrees instead of DDMM.hh, missing symbol table bytes, ...), which this
// domain must reject cleanly - never panic, never emit a bad envelope -
// rather than error loudly on what is, for APRS-IS, entirely normal noise.
func TestFixtureLiveCapture(t *testing.T) {
	file, err := os.Open(filepath.Join("testdata", "live_capture_2026-08-28.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	d := New()
	var total, emitted, hardErrors int
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		total++
		payload, err := json.Marshal(rawLine{Raw: line})
		if err != nil {
			t.Fatal(err)
		}
		record := plugins.RawRecord{
			SourcePluginID:   "aprsis",
			SourceInstanceID: "fixture",
			OriginalID:       "fixture",
			Domain:           "aprs",
			ObservedUTC:      time.Now().UTC(),
			Payload:          payload,
		}
		messages, err := d.Normalize(context.Background(), record)
		if err != nil {
			hardErrors++
			t.Errorf("line %d: unexpected hard error on %q: %v", total, line, err)
			continue
		}
		for _, m := range messages {
			if err := m.Validate(); err != nil {
				t.Errorf("line %d: envelope failed canonical validation: %v (line: %q)", total, err, line)
			}
		}
		if len(messages) > 0 {
			emitted++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	rate := float64(emitted) / float64(total) * 100
	t.Logf("fixture replay: %d real captured lines, %d produced an envelope (%.1f%%), %d hard errors", total, emitted, rate, hardErrors)

	if total < 300 {
		t.Fatalf("fixture is smaller than expected: %d lines", total)
	}
	if hardErrors > 0 {
		t.Fatalf("%d lines caused a hard Normalize error; malformed input must be skipped, not errored", hardErrors)
	}
	// Measured at 64.8% for this exact corpus; a floor well below that
	// catches a real regression (e.g. positions or objects breaking
	// wholesale) without being sensitive to the fixture's exact makeup.
	const minRate = 50.0
	if rate < minRate {
		t.Fatalf("decode rate %.1f%% is well below the expected floor of %.0f%%", rate, minRate)
	}
}
