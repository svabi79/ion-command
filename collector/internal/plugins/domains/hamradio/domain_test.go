package hamradio

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestSpotNormalizesThroughCanonicalModel(t *testing.T) {
	payload, _ := json.Marshal(rawSpot{SpotID: "one", TXCallsign: "N0CALL", RXCallsign: "TEST1", TXLongitude: 8, TXLatitude: 47, RXLongitude: -74, RXLatitude: 41, FrequencyHz: 14074000, Band: "20m", Mode: "FT8", SNRDb: -11})
	record := plugins.RawRecord{SourcePluginID: "mock.radio", SourceInstanceID: "test", OriginalID: "one", Domain: "hamradio", ObservedUTC: time.Now(), Payload: payload}
	domain := New()
	messages, err := domain.Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("wanted two entities and one relationship, got %d", len(messages))
	}
	link := messages[2]
	if link.SemanticType != "radio.reception" || link.Geometry.Type != "GreatCircle" {
		t.Fatalf("unexpected link: %#v", link)
	}
	if link.Properties["display.title"] != "Observed Link" || link.Properties["display.from"] != "N0CALL" || link.Properties["display.to"] != "TEST1" {
		t.Fatalf("missing domain-supplied generic display metadata: %#v", link.Properties)
	}
	if err := link.Validate(); err != nil {
		t.Fatalf("invalid canonical link: %v", err)
	}
	repeated, err := domain.Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated) != 1 || repeated[0].SemanticType != "radio.reception" {
		t.Fatalf("repeat should only emit the relationship, got %#v", repeated)
	}
	if _, present := link.Properties["display.fromRegion"]; present {
		t.Fatalf("region metadata must be absent without dxcc codes: %#v", link.Properties)
	}
}

func TestSpotCarriesDxccRegions(t *testing.T) {
	tx, rx := 287, 291
	payload, _ := json.Marshal(rawSpot{SpotID: "two", TXCallsign: "HB9ABC", RXCallsign: "K1ABC", TXLongitude: 8, TXLatitude: 47, RXLongitude: -74, RXLatitude: 41, FrequencyHz: 14074000, Band: "20m", Mode: "FT8", SNRDb: -3, TXDxcc: &tx, RXDxcc: &rx})
	record := plugins.RawRecord{SourcePluginID: "pskreporter", SourceInstanceID: "test", OriginalID: "two", Domain: "hamradio", ObservedUTC: time.Now(), Payload: payload}
	messages, err := New().Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	link := messages[len(messages)-1]
	if link.Properties["display.fromRegion"] != "Switzerland" || link.Properties["display.toRegion"] != "United States" {
		t.Fatalf("unexpected region metadata: %#v", link.Properties)
	}
	if link.Properties["txDxcc"] != 287 || link.Properties["rxDxcc"] != 291 {
		t.Fatalf("unexpected dxcc codes: %#v", link.Properties)
	}
	unknown := 9999
	zero := 0
	payload, _ = json.Marshal(rawSpot{SpotID: "three", TXCallsign: "A1A", RXCallsign: "B2B", TXLongitude: 1, TXLatitude: 1, RXLongitude: 2, RXLatitude: 2, FrequencyHz: 7074000, Band: "40m", Mode: "FT8", TXDxcc: &unknown, RXDxcc: &zero})
	record.OriginalID = "three"
	record.Payload = payload
	messages, err = New().Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	link = messages[len(messages)-1]
	if link.Properties["display.fromRegion"] != "DXCC 9999" {
		t.Fatalf("unknown code should fall back to numeric label: %#v", link.Properties)
	}
	if _, present := link.Properties["display.toRegion"]; present {
		t.Fatalf("code 0 must not produce a region: %#v", link.Properties)
	}
}
