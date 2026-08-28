package ais

import (
	"encoding/json"
	"os"
	"testing"
)

func loadFrame(t *testing.T, name string) []byte {
	t.Helper()
	frame, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func decodeOne(t *testing.T, name string) map[string]any {
	t.Helper()
	records, err := ParseFrame(loadFrame(t, name), "test")
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", name, err)
	}
	if len(records) != 1 {
		t.Fatalf("%s: expected exactly one record, got %d", name, len(records))
	}
	var payload map[string]any
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if records[0].Domain != "maritime" || records[0].SourcePluginID != "ais" {
		t.Fatalf("%s: unexpected routing: %+v", name, records[0])
	}
	return payload
}

// testdata/position_report.json reproduces the worked PositionReport example
// published on https://aisstream.io/documentation (MMSI 368207620, "EXAMPLE
// VESSEL", a fix in the Miami area, Sog 12.4 kn, Cog 86.7°, TrueHeading 87°),
// completed with the remaining schema-required fields the docs page's own
// abbreviated example elided (RepeatIndicator, NavigationalStatus, etc., per
// the OpenAPI model at github.com/aisstream/ais-message-models).
func TestParsePositionReportRealisticCoordinates(t *testing.T) {
	payload := decodeOne(t, "position_report.json")
	if payload["kind"] != "position" {
		t.Fatalf("unexpected kind: %v", payload["kind"])
	}
	if payload["mmsi"].(float64) != 368207620 {
		t.Fatalf("unexpected mmsi: %v", payload["mmsi"])
	}
	if payload["classB"] != false {
		t.Fatalf("expected Class A, got classB=%v", payload["classB"])
	}
	lat, lon := payload["lat"].(float64), payload["lon"].(float64)
	if lat < 25.0 || lat > 26.0 || lon < -81.0 || lon > -80.0 {
		t.Fatalf("position off (expected Miami area): lat=%v lon=%v", lat, lon)
	}
	if payload["sogKn"].(float64) != 12.4 {
		t.Fatalf("unexpected sog: %v", payload["sogKn"])
	}
	if payload["cogDeg"].(float64) != 86.7 {
		t.Fatalf("unexpected cog: %v", payload["cogDeg"])
	}
	if payload["headingDeg"].(float64) != 87 {
		t.Fatalf("unexpected heading: %v", payload["headingDeg"])
	}
	if payload["navStatus"].(float64) != 0 {
		t.Fatalf("unexpected navStatus: %v", payload["navStatus"])
	}
	if payload["metaShipName"] != "EXAMPLE VESSEL" {
		t.Fatalf("unexpected metaShipName: %v", payload["metaShipName"])
	}
}

// testdata/class_b_position_report.json carries the AIS "not available"
// sentinels for course (360.0) and heading (511) exactly as a real Class B
// unit without a heading sensor would send them, and has no navStatus/
// rateOfTurn key at all — StandardClassBPositionReport does not carry those
// fields, they are not merely blanked out.
func TestParseClassBPositionHasNoNavStatusField(t *testing.T) {
	payload := decodeOne(t, "class_b_position_report.json")
	if payload["classB"] != true {
		t.Fatalf("expected Class B, got classB=%v", payload["classB"])
	}
	if _, present := payload["navStatus"]; present {
		t.Fatalf("Class B record must not carry a navStatus key: %v", payload)
	}
	if _, present := payload["rateOfTurn"]; present {
		t.Fatalf("Class B record must not carry a rateOfTurn key: %v", payload)
	}
	if payload["cogDeg"].(float64) != 360.0 {
		t.Fatalf("unexpected cog: %v", payload["cogDeg"])
	}
	if payload["headingDeg"].(float64) != 511 {
		t.Fatalf("unexpected heading: %v", payload["headingDeg"])
	}
}

// testdata/position_report_sentinels.json sets every AIS "not available"
// sentinel at once (Longitude 181, Latitude 91, Sog 102.3, Cog 360.0,
// TrueHeading 511, RateOfTurn -128, NavigationalStatus 15) to prove the
// decoder passes them through unchanged rather than interpreting them —
// interpretation is the maritime domain's job (see domain_test.go).
func TestParsePositionReportSentinelsPassThroughUnchanged(t *testing.T) {
	payload := decodeOne(t, "position_report_sentinels.json")
	if payload["lon"].(float64) != 181 || payload["lat"].(float64) != 91 {
		t.Fatalf("sentinel position must pass through unchanged: %v", payload)
	}
	if payload["sogKn"].(float64) != 102.3 {
		t.Fatalf("sentinel sog must pass through unchanged: %v", payload["sogKn"])
	}
	if payload["cogDeg"].(float64) != 360.0 {
		t.Fatalf("sentinel cog must pass through unchanged: %v", payload["cogDeg"])
	}
	if payload["headingDeg"].(float64) != 511 {
		t.Fatalf("sentinel heading must pass through unchanged: %v", payload["headingDeg"])
	}
	if payload["rateOfTurn"].(float64) != -128 {
		t.Fatalf("sentinel rate of turn must pass through unchanged: %v", payload["rateOfTurn"])
	}
	if payload["navStatus"].(float64) != 15 {
		t.Fatalf("unexpected navStatus: %v", payload["navStatus"])
	}
}

func TestParseShipStaticData(t *testing.T) {
	payload := decodeOne(t, "ship_static_data.json")
	if payload["kind"] != "static" {
		t.Fatalf("unexpected kind: %v", payload["kind"])
	}
	if payload["mmsi"].(float64) != 368207620 {
		t.Fatalf("unexpected mmsi: %v", payload["mmsi"])
	}
	if payload["name"] != "EXAMPLE VESSEL" || payload["callSign"] != "WDF1234" {
		t.Fatalf("unexpected identity fields: %v", payload)
	}
	if payload["shipTypeCode"].(float64) != 70 {
		t.Fatalf("unexpected shipTypeCode: %v", payload["shipTypeCode"])
	}
	if payload["destination"] != "ROTTERDAM" {
		t.Fatalf("unexpected destination: %v", payload["destination"])
	}
	if payload["etaMonth"].(float64) != 8 || payload["etaDay"].(float64) != 30 {
		t.Fatalf("unexpected eta: %v", payload)
	}
	if payload["dimA"].(float64) != 100 || payload["dimB"].(float64) != 20 {
		t.Fatalf("unexpected dimensions: %v", payload)
	}
}

// The Class B static/voyage report splits across two independent messages;
// Part A carries only the name, Part B carries type/call sign/dimensions.
func TestParseStaticDataReportPartsCarryDisjointFields(t *testing.T) {
	partA := decodeOne(t, "static_data_report_part_a.json")
	if partA["name"] != "EXAMPLE YACHT" {
		t.Fatalf("part A must carry the name: %v", partA)
	}
	if _, present := partA["callSign"]; present {
		t.Fatalf("part A must not carry call sign: %v", partA)
	}

	partB := decodeOne(t, "static_data_report_part_b.json")
	if _, present := partB["name"]; present {
		t.Fatalf("part B must not carry the name: %v", partB)
	}
	if partB["callSign"] != "2ABC3" {
		t.Fatalf("part B must carry the call sign: %v", partB)
	}
	if partB["shipTypeCode"].(float64) != 37 {
		t.Fatalf("unexpected shipTypeCode: %v", partB)
	}
	if partB["dimA"].(float64) != 8 {
		t.Fatalf("unexpected dimensions: %v", partB)
	}
}

// A base station (AIS message type 4) is never in FilterMessageTypes, so a
// well-behaved server never sends one — but this proves the decoder itself
// also does not manufacture a record for a message type it does not model,
// rather than relying solely on the server-side filter.
func TestParseIgnoresUnsubscribedMessageTypes(t *testing.T) {
	for _, name := range []string{"base_station_report.json", "subscription_confirmation.json"} {
		records, err := ParseFrame(loadFrame(t, name), "test")
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if len(records) != 0 {
			t.Fatalf("%s: expected no records, got %d", name, len(records))
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := ParseFrame([]byte("not json at all"), "test"); err == nil {
		t.Fatal("garbage frame must be rejected")
	}
}
