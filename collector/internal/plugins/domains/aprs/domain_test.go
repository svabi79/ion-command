package aprs

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestNormalizePlainPosition(t *testing.T) {
	d := New()
	msgs, err := d.Normalize(context.Background(), rawRecordFor(t, "N0CALL>APRS,TCPIP*:!4903.50N/07201.75W>Test comment"))
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(msgs))
	}
	m := msgs[0]
	if m.EntityID != "aprs:station:N0CALL" {
		t.Fatalf("entityID = %q", m.EntityID)
	}
	if m.SemanticType != "aprs.station" {
		t.Fatalf("semanticType = %q", m.SemanticType)
	}
	if m.Domain != "aprs" {
		t.Fatalf("domain = %q", m.Domain)
	}
	var point []float64
	if err := json.Unmarshal(m.Geometry.Coordinates, &point); err != nil {
		t.Fatal(err)
	}
	if !almostEqual(point[0], -(72+1.75/60), 1e-9) || !almostEqual(point[1], 49+3.50/60, 1e-9) {
		t.Fatalf("geometry = %v (want lon,lat order)", point)
	}
	if m.Properties["display.title"] != "N0CALL" {
		t.Fatalf("display.title = %v", m.Properties["display.title"])
	}
	if m.Properties["display.primary"] != "Car" {
		t.Fatalf("display.primary = %v, want Car", m.Properties["display.primary"])
	}
	if m.Properties["display.secondary"] != "Test comment" {
		t.Fatalf("display.secondary = %v", m.Properties["display.secondary"])
	}
	if m.Time.ValidUntilUTC == nil {
		t.Fatal("expected a validity horizon")
	}
	if got := m.Time.ValidUntilUTC.Sub(m.Time.ObservedUTC); got != positionValidity {
		t.Fatalf("validity window = %v, want %v", got, positionValidity)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("envelope must validate against the canonical schema: %v", err)
	}
}

func rawRecordFor(t *testing.T, line string) plugins.RawRecord {
	t.Helper()
	payload, err := json.Marshal(rawLine{Raw: line})
	if err != nil {
		t.Fatal(err)
	}
	return plugins.RawRecord{
		SourcePluginID:   "aprsis",
		SourceInstanceID: "test",
		OriginalID:       "aprsis-test-1",
		Domain:           "aprs",
		ObservedUTC:      time.Now().UTC(),
		Payload:          payload,
	}
}

func TestNormalizeUnsupportedTypesSkipCleanly(t *testing.T) {
	d := New()
	lines := []string{
		"N0CALL>APRS::TARGET   :a message",
		"N0CALL>APRS:>a status report",
		"N0CALL>APRS:T#123,456,789,012,345,678,00000000",
		"N0CALL>APRS:_10090556c220s004g005t077r000p000P000h50b09900wRSW",
		"N0CALL>APRS:}third>party:!4903.50N/07201.75W-",
		"not even a valid tnc2 line",
	}
	for _, line := range lines {
		msgs, err := d.Normalize(context.Background(), rawRecordFor(t, line))
		if err != nil {
			t.Fatalf("%q: unsupported/invalid input must not be a hard error, got %v", line, err)
		}
		if len(msgs) != 0 {
			t.Fatalf("%q: expected no envelopes, got %d", line, len(msgs))
		}
	}
}

func TestNormalizeDedupUnchangedPosition(t *testing.T) {
	d := New()
	line := "N0CALL>APRS,TCPIP*:!4903.50N/07201.75W>first"
	first, err := d.Normalize(context.Background(), rawRecordFor(t, line))
	if err != nil || len(first) != 1 {
		t.Fatalf("first report should emit, got %d, err=%v", len(first), err)
	}
	// Same position, different path/comment (as a re-delivered or looped
	// packet commonly is) must not produce a second envelope.
	dup := "N0CALL>APRS,WIDE1-1,WIDE2-1*:!4903.50N/07201.75W>looped via a different path"
	second, err := d.Normalize(context.Background(), rawRecordFor(t, dup))
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("expected the duplicate position to be suppressed, got %d envelopes", len(second))
	}
	moved := "N0CALL>APRS,TCPIP*:!4904.50N/07201.75W>moved"
	third, err := d.Normalize(context.Background(), rawRecordFor(t, moved))
	if err != nil || len(third) != 1 {
		t.Fatalf("a real position change must emit, got %d, err=%v", len(third), err)
	}
}

func TestNormalizeObjectKillExpiresImmediately(t *testing.T) {
	d := New()
	live := ";LEADER   *092345z4903.50N/07201.75W>088/036"
	msgs, err := d.Normalize(context.Background(), rawRecordFor(t, "N0CALL>APRS,TCPIP*:"+live))
	if err != nil || len(msgs) != 1 {
		t.Fatalf("live object should emit, got %d, err=%v", len(msgs), err)
	}
	if !msgs[0].Time.ValidUntilUTC.After(msgs[0].Time.ObservedUTC) {
		t.Fatal("a live object should have a real validity window")
	}

	killed := ";LEADER    092345z4903.50N/07201.75W>088/036"
	killMsgs, err := d.Normalize(context.Background(), rawRecordFor(t, "N0CALL>APRS,TCPIP*:"+killed))
	if err != nil || len(killMsgs) != 1 {
		t.Fatalf("a kill report at the SAME position must still emit (not deduped), got %d, err=%v", len(killMsgs), err)
	}
	if killMsgs[0].Time.ValidUntilUTC.After(killMsgs[0].Time.ObservedUTC) {
		t.Fatal("a killed object must expire immediately")
	}
	if killMsgs[0].Properties["live"] != false {
		t.Fatalf("live property = %v, want false", killMsgs[0].Properties["live"])
	}
}

func TestNormalizeEmergencyMicEHighlighted(t *testing.T) {
	d := New()
	// Destination "234567": message bits all zero -> Emergency.
	line := "N0CALL>234567,TCPIP*:`(5NXXXj/Help"
	msgs, err := d.Normalize(context.Background(), rawRecordFor(t, line))
	if err != nil || len(msgs) != 1 {
		t.Fatalf("expected 1 envelope, got %d, err=%v", len(msgs), err)
	}
	props := msgs[0].Properties
	if props["visual.emergency"] != "EMERGENCY" {
		t.Fatalf("visual.emergency = %v", props["visual.emergency"])
	}
	if props["display.title"] != "N0CALL  //  EMERGENCY" {
		t.Fatalf("display.title = %v", props["display.title"])
	}
}

func TestNormalizeInvalidJSONPayloadIsAnError(t *testing.T) {
	d := New()
	_, err := d.Normalize(context.Background(), plugins.RawRecord{Payload: []byte("not json")})
	if err == nil {
		t.Fatal("a malformed record payload (a real bug, not ordinary traffic) must be reported as an error")
	}
}

// TestNormalizeConcurrentAccess exercises the dedup cache from many
// goroutines at once, matching how the pipeline actually calls Normalize
// (several workers share one Domain instance). Run with -race; this test
// alone proves nothing without it.
func TestNormalizeConcurrentAccess(t *testing.T) {
	d := New()
	const goroutines = 16
	const perGoroutine = 200
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				call := fmt.Sprintf("W%dCALL-%d", g, i%5)
				// "49MM.hh" shape (degrees fixed at 49, minutes cycling) -
				// same station reporting a genuinely changing position, to
				// exercise both the "changed" and "unchanged" dedup paths.
				line := fmt.Sprintf("%s>APRS,TCPIP*:!49%02d.00N/07201.75W>concurrent", call, i%60)
				if _, err := d.Normalize(context.Background(), rawRecordFor(t, line)); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestDomainIdentity(t *testing.T) {
	d := New()
	if d.ID() != "domain.aprs" {
		t.Fatalf("ID() = %q", d.ID())
	}
	if d.Domain() != "aprs" {
		t.Fatalf("Domain() = %q", d.Domain())
	}
}
