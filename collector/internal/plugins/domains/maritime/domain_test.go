package maritime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/ais"
)

// loadFixture decodes a real aisstream.io-shaped frame through the actual
// source decoder (ais.ParseFrame), so these domain tests exercise the same
// two-stage pipeline production traffic goes through, using fixtures
// grounded in the provider's published documentation and schema — see
// collector/internal/plugins/sources/ais/testdata for their provenance.
func loadFixture(t *testing.T, name string) plugins.RawRecord {
	t.Helper()
	frame, err := os.ReadFile("../../sources/ais/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	records, err := ais.ParseFrame(frame, "test")
	if err != nil {
		t.Fatalf("%s: decode failed: %v", name, err)
	}
	if len(records) != 1 {
		t.Fatalf("%s: expected exactly one raw record, got %d", name, len(records))
	}
	return records[0]
}

func normalize(t *testing.T, domain *Domain, record plugins.RawRecord) []events.Envelope {
	t.Helper()
	messages, err := domain.Normalize(context.Background(), record)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	return messages
}

func TestPositionDecodingAtRealisticCoordinates(t *testing.T) {
	domain := New()
	record := loadFixture(t, "position_report.json")
	messages := normalize(t, domain, record)
	if len(messages) != 1 {
		t.Fatalf("expected one envelope, got %d", len(messages))
	}
	event := messages[0]
	if event.SemanticType != "maritime.vessel" || event.Domain != "maritime" {
		t.Fatalf("unexpected routing: %+v", event)
	}
	if event.EntityID != "maritime:vessel:368207620" {
		t.Fatalf("unexpected entity id: %s", event.EntityID)
	}
	var point []float64
	if err := json.Unmarshal(event.Geometry.Coordinates, &point); err != nil {
		t.Fatal(err)
	}
	if len(point) < 2 || point[0] < -81 || point[0] > -80 || point[1] < 25 || point[1] > 26 {
		t.Fatalf("unexpected coordinates (expected Miami area): %v", point)
	}
	// No static data has been joined in this call at all, but aisstream
	// repeats a best-effort ShipName in MetaData on every message, and the
	// domain uses it as a fallback title until the real static data joins.
	if event.Properties["display.title"] != "EXAMPLE VESSEL" {
		t.Fatalf("expected metaShipName fallback title, got %v", event.Properties["display.title"])
	}
	if _, present := event.Properties["vesselType"]; present {
		t.Fatalf("vessel type must not appear before static data has joined: %v", event.Properties["vesselType"])
	}
}

// The real AIS pitfall this test targets: a vessel's name/type/destination
// live in an entirely separate, much less frequent message (ShipStaticData)
// than its position (PositionReport). The domain must cache the static
// message and join it onto later position updates for the same MMSI.
func TestStaticDataJoinsOntoLaterPositionReports(t *testing.T) {
	domain := New()

	// Position arrives first, before any static data is known. The title
	// already shows via MetaData's best-effort ShipName echo, but fields
	// that only static/voyage messages carry must still be absent.
	before := normalize(t, domain, loadFixture(t, "position_report.json"))[0]
	for _, key := range []string{"vesselType", "callSign", "display.tertiary"} {
		if _, present := before.Properties[key]; present {
			t.Fatalf("property %q must not appear before static data has joined: %v", key, before.Properties[key])
		}
	}

	// Static/voyage data arrives; it must not itself produce an envelope
	// (there is no position in a ShipStaticData message).
	staticMessages := normalize(t, domain, loadFixture(t, "ship_static_data.json"))
	if len(staticMessages) != 0 {
		t.Fatalf("a static-only record must not emit an envelope, got %d", len(staticMessages))
	}

	// A later position report for the same MMSI (must differ from the first
	// so it is not suppressed as a duplicate) now carries the joined name,
	// type, call sign, destination and ETA.
	moved := mutatePositionPayload(t, "position_report.json", func(p map[string]any) {
		p["lat"] = 25.8000
	})
	after := normalize(t, domain, moved)[0]
	if after.Properties["display.title"] != "EXAMPLE VESSEL" {
		t.Fatalf("expected joined name, got %v", after.Properties["display.title"])
	}
	if after.Properties["vesselType"] != "Cargo" {
		t.Fatalf("expected joined vessel type Cargo, got %v", after.Properties["vesselType"])
	}
	if after.Properties["callSign"] != "WDF1234" {
		t.Fatalf("expected joined call sign, got %v", after.Properties["callSign"])
	}
	if after.Properties["flagState"] != "United States" {
		t.Fatalf("expected MID 368 to resolve to United States, got %v", after.Properties["flagState"])
	}
	tertiary, _ := after.Properties["display.tertiary"].(string)
	if tertiary != "ROTTERDAM  //  ETA 08-30 14:30Z" {
		t.Fatalf("unexpected destination/ETA line: %q", tertiary)
	}
}

// Class B static data arrives as two independent part-messages; the cache
// must merge both rather than the second part erasing the first.
func TestStaticDataReportPartsMergeAcrossMessages(t *testing.T) {
	domain := New()
	normalize(t, domain, loadFixture(t, "static_data_report_part_a.json")) // Name only
	normalize(t, domain, loadFixture(t, "static_data_report_part_b.json")) // Type/CallSign/Dimension only

	event := normalize(t, domain, loadFixture(t, "class_b_position_report.json"))[0]
	if event.Properties["display.title"] != "EXAMPLE YACHT" {
		t.Fatalf("expected name from part A to survive part B, got %v", event.Properties["display.title"])
	}
	if event.Properties["callSign"] != "2ABC3" {
		t.Fatalf("expected call sign from part B, got %v", event.Properties["callSign"])
	}
	if event.Properties["vesselType"] != "Pleasure Craft" {
		t.Fatalf("expected vessel type from part B, got %v", event.Properties["vesselType"])
	}
}

// Every AIS "not available" sentinel at once: position, speed, course,
// heading, rate of turn. A missing fix must not become a null-island entity;
// the other sentinel fields must simply be absent from the properties.
func TestUnavailableSentinelsAreRecognised(t *testing.T) {
	domain := New()
	messages := normalize(t, domain, loadFixture(t, "position_report_sentinels.json"))
	if len(messages) != 0 {
		t.Fatalf("a position-not-available record must not emit an envelope, got %d", len(messages))
	}
}

func TestSpeedCourseHeadingSentinelsOmitted(t *testing.T) {
	domain := New()
	// Reuse the realistic fixture but overwrite only the fields under test,
	// keeping a valid position so the envelope is still emitted.
	record := mutatePositionPayload(t, "position_report.json", func(p map[string]any) {
		p["sogKn"] = 102.3
		p["cogDeg"] = 360.0
		p["headingDeg"] = 511.0
		p["rateOfTurn"] = -128.0
	})
	event := normalize(t, domain, record)[0]
	for _, key := range []string{"speedKn", "courseDeg", "headingDeg", "visual.headingDeg", "visual.speedMps", "rateOfTurnCode"} {
		if _, present := event.Properties[key]; present {
			t.Fatalf("sentinel value for %q must not appear in properties: %v", key, event.Properties[key])
		}
	}
}

// A known (non-sentinel) rate of turn is passed through as a labelled raw
// code, not silently dropped.
func TestKnownRateOfTurnIsExposed(t *testing.T) {
	domain := New()
	event := normalize(t, domain, loadFixture(t, "position_report.json"))[0]
	code, ok := event.Properties["rateOfTurnCode"].(int)
	if !ok || code != 0 {
		t.Fatalf("expected rateOfTurnCode 0 from the fixture, got %v", event.Properties["rateOfTurnCode"])
	}
}

// mmsiCategory table, exercised through the public Normalize entry point:
// base stations, aids to navigation, and SAR aircraft must never become
// vessel entities even if a position-shaped record carries their MMSI —
// defence in depth beyond the source's own message-type filtering.
func TestNonVesselMMSIsAreRejected(t *testing.T) {
	cases := []struct {
		name string
		mmsi int64
	}{
		{"base_station", 2241023}, // 00-prefixed once zero-padded to 9 digits
		{"aid_to_navigation", 992351234},
		{"sar_aircraft", 111241023},
		{"auxiliary_craft", 982351234},
		{"group_ship_call", 235012345 % 100000000}, // leading zero once padded
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			domain := New()
			record := mutatePositionPayload(t, "position_report.json", func(p map[string]any) {
				p["mmsi"] = tc.mmsi
			})
			messages := normalize(t, domain, record)
			if len(messages) != 0 {
				t.Fatalf("mmsi %d (%s) must not become a vessel entity, got %d envelopes", tc.mmsi, tc.name, len(messages))
			}
		})
	}
}

func TestGenuineVesselMMSIRangeIsAccepted(t *testing.T) {
	for _, mmsi := range []int64{200000000, 368207620, 799999999} {
		if mmsiCategory(mmsi) != "vessel" {
			t.Fatalf("mmsi %d should classify as a vessel, got %q", mmsi, mmsiCategory(mmsi))
		}
	}
}

// The core "moving vs. moored" requirement: an unchanged fix must not
// re-emit on every message, but a real change (or enough elapsed time) must.
func TestDuplicatePositionsAreSuppressed(t *testing.T) {
	domain := New()
	first := normalize(t, domain, loadFixture(t, "position_report.json"))
	if len(first) != 1 {
		t.Fatalf("expected the first sighting to emit, got %d", len(first))
	}
	// The exact same fix again immediately after: no movement, no status
	// change, well inside the reaffirm window — must be suppressed.
	repeat := normalize(t, domain, loadFixture(t, "position_report.json"))
	if len(repeat) != 0 {
		t.Fatalf("an unchanged position must not re-emit, got %d envelopes", len(repeat))
	}
	// A materially different position for the same vessel must emit again.
	moved := mutatePositionPayload(t, "position_report.json", func(p map[string]any) {
		p["lat"] = 26.5
	})
	changed := normalize(t, domain, moved)
	if len(changed) != 1 {
		t.Fatalf("a changed position must emit, got %d envelopes", len(changed))
	}
}

// A moored vessel keeps confirming the identical position every few minutes.
// Pure fingerprint-based dedup would let its entity age past ValidUntilUTC
// and vanish from the globe between genuine changes; the domain must
// re-affirm periodically to keep the validity window sliding forward.
func TestUnchangedPositionReaffirmsBeforeGoingStale(t *testing.T) {
	domain := New()
	base := decodeFixturePayload(t, "position_report.json")
	base["navStatus"] = 5.0 // Moored -> the long (20 minute) validity window

	first := normalizeRawPayload(t, domain, base, time.Unix(1_700_000_000, 0).UTC())
	if len(first) != 1 {
		t.Fatalf("expected the first sighting to emit, got %d", len(first))
	}
	validUntil := *first[0].Time.ValidUntilUTC

	// Same fingerprint, but observed well past the reaffirm point (half the
	// validity window) — must re-emit so the marker's validity keeps sliding
	// forward, even though nothing about the vessel actually changed.
	later := normalizeRawPayload(t, domain, base, validUntil.Add(-1*time.Minute))
	if len(later) != 1 {
		t.Fatalf("an unchanged moored position must reaffirm before going stale, got %d envelopes", len(later))
	}
	if !later[0].Time.ValidUntilUTC.After(validUntil) {
		t.Fatalf("reaffirmation must push validity forward: first=%v later=%v", validUntil, *later[0].Time.ValidUntilUTC)
	}
}

// The static/position join cache is a bounded map (HARD RULES: every queue,
// cache and map is bounded). This drives a domain with a tiny bound far past
// capacity and checks the map itself never grows past it — the eviction
// path, not just its absence of a crash.
func TestBoundedCacheEviction(t *testing.T) {
	domain := newDomain(10)
	for i := 0; i < 500; i++ {
		mmsi := int64(200000001 + i)
		record := mutatePositionPayload(t, "position_report.json", func(p map[string]any) {
			p["mmsi"] = mmsi
			p["lat"] = 10.0 + float64(i)*0.001
		})
		normalize(t, domain, record)
		domain.mu.Lock()
		size := len(domain.vessels)
		domain.mu.Unlock()
		if size > domain.maxVessels {
			t.Fatalf("cache grew past its bound: size=%d bound=%d (after %d insertions)", size, domain.maxVessels, i+1)
		}
	}
	domain.mu.Lock()
	finalSize := len(domain.vessels)
	domain.mu.Unlock()
	if finalSize == 0 {
		t.Fatal("cache should not have evicted everything")
	}
	if finalSize > domain.maxVessels {
		t.Fatalf("final cache size %d exceeds bound %d", finalSize, domain.maxVessels)
	}
}

// The pipeline runs several worker goroutines against the same registered
// Domain instance (see collector/internal/pipeline), so the shared vessel
// cache genuinely is accessed concurrently in production, not just here
// hypothetically. Run with -race to actually prove it.
func TestConcurrentNormalizeIsRaceFree(t *testing.T) {
	domain := newDomain(50)
	position := decodeFixturePayload(t, "position_report.json")
	static := decodeFixturePayload(t, "ship_static_data.json")

	var wg sync.WaitGroup
	for worker := 0; worker < 20; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				mmsi := int64(200000001 + (worker*100+i)%37)
				pos := clonePayload(position)
				pos["mmsi"] = mmsi
				pos["lat"] = 10.0 + float64(i)*0.0001
				normalizeRawPayload(t, domain, pos, time.Now().UTC())

				st := clonePayload(static)
				st["mmsi"] = mmsi
				normalizeRawPayload(t, domain, st, time.Now().UTC())
			}
		}(worker)
	}
	wg.Wait()

	domain.mu.Lock()
	size := len(domain.vessels)
	domain.mu.Unlock()
	if size == 0 || size > domain.maxVessels {
		t.Fatalf("unexpected cache size after concurrent access: %d (bound %d)", size, domain.maxVessels)
	}
}

func clonePayload(payload map[string]any) map[string]any {
	clone := make(map[string]any, len(payload))
	for k, v := range payload {
		clone[k] = v
	}
	return clone
}

func TestMalformedRecordIsRejected(t *testing.T) {
	domain := New()
	_, err := domain.Normalize(context.Background(), plugins.RawRecord{
		Payload: json.RawMessage(`{"kind":"position","mmsi":0}`),
	})
	if err == nil {
		t.Fatal("mmsi <= 0 must be rejected as malformed")
	}
	if _, err := domain.Normalize(context.Background(), plugins.RawRecord{Payload: json.RawMessage(`not json`)}); err == nil {
		t.Fatal("invalid JSON must be rejected")
	}
}

// --- helpers -----------------------------------------------------------

func decodeFixturePayload(t *testing.T, name string) map[string]any {
	t.Helper()
	record := loadFixture(t, name)
	var payload map[string]any
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

// mutatePositionPayload loads a real fixture, decodes it to a generic map,
// applies a targeted change, and re-encodes it as a RawRecord — used to
// exercise cases (a changed position, a swapped-in non-vessel MMSI) that a
// single static fixture file cannot cover by itself, while still starting
// from a schema-real message rather than a hand-built one.
func mutatePositionPayload(t *testing.T, fixture string, mutate func(map[string]any)) plugins.RawRecord {
	t.Helper()
	payload := decodeFixturePayload(t, fixture)
	mutate(payload)
	return encodeRawRecord(t, payload, time.Now().UTC())
}

func normalizeRawPayload(t *testing.T, domain *Domain, payload map[string]any, observed time.Time) []events.Envelope {
	t.Helper()
	record := encodeRawRecord(t, payload, observed)
	messages, err := domain.Normalize(context.Background(), record)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	return messages
}

func encodeRawRecord(t *testing.T, payload map[string]any, observed time.Time) plugins.RawRecord {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return plugins.RawRecord{
		SourcePluginID:   "ais",
		SourceInstanceID: "test",
		OriginalID:       fmt.Sprintf("ais-test-%v-%d", payload["mmsi"], observed.UnixNano()),
		Domain:           "maritime",
		ObservedUTC:      observed,
		Payload:          encoded,
	}
}
