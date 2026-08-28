package hamradio

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestSpotNormalizesThroughCanonicalModel(t *testing.T) {
	snr := -11
	payload, _ := json.Marshal(rawSpot{SpotID: "one", TXCallsign: "N0CALL", RXCallsign: "TEST1", TXLongitude: 8, TXLatitude: 47, RXLongitude: -74, RXLatitude: 41, FrequencyHz: 14074000, Band: "20m", Mode: "FT8", SNRDb: &snr})
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
	snr := -3
	payload, _ := json.Marshal(rawSpot{SpotID: "two", TXCallsign: "HB9ABC", RXCallsign: "K1ABC", TXLongitude: 8, TXLatitude: 47, RXLongitude: -74, RXLatitude: 41, FrequencyHz: 14074000, Band: "20m", Mode: "FT8", SNRDb: &snr, TXDxcc: &tx, RXDxcc: &rx})
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

// TestSpotWithoutSignalReportOmitsSNR covers the DX cluster shape: a
// human-typed spot frequently carries no signal report and no recognisable
// mode. Both must be omitted rather than rendered as a fabricated 0 dB / a
// blank segment in the primary label.
func TestSpotWithoutSignalReportOmitsSNR(t *testing.T) {
	payload, _ := json.Marshal(rawSpot{SpotID: "four", TXCallsign: "9M26MA", RXCallsign: "RU3GC", TXLongitude: 101.7, TXLatitude: 3.1, RXLongitude: 37.6, RXLatitude: 55.7, FrequencyHz: 21240000, Band: "15m"})
	record := plugins.RawRecord{SourcePluginID: "dxcluster", SourceInstanceID: "test", OriginalID: "four", Domain: "hamradio", ObservedUTC: time.Now(), Payload: payload}
	messages, err := New().Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	link := messages[len(messages)-1]
	if _, present := link.Properties["snrDb"]; present {
		t.Fatalf("snrDb must be absent, not fabricated as zero: %#v", link.Properties)
	}
	if _, present := link.Properties["display.secondary"]; present {
		t.Fatalf("display.secondary must be absent when there is nothing to show: %#v", link.Properties)
	}
	if link.Properties["display.primary"] != "15m  //  21.240 MHz" {
		t.Fatalf("blank mode must not leave an empty segment: %q", link.Properties["display.primary"])
	}
	if err := link.Validate(); err != nil {
		t.Fatalf("invalid canonical link: %v", err)
	}
}

// TestSpotUsesPlainRegionFallback covers the RBN / DX cluster / WSPR shape:
// a region name resolved by the source itself (country file or Maidenhead
// lookup) rather than an ADIF DXCC code.
func TestSpotUsesPlainRegionFallback(t *testing.T) {
	payload, _ := json.Marshal(rawSpot{SpotID: "five", TXCallsign: "HB9ABC", RXCallsign: "K1ABC", TXLongitude: 8, TXLatitude: 47, RXLongitude: -74, RXLatitude: 41, FrequencyHz: 14097100, Band: "20m", TXRegion: "Switzerland", RXRegion: "United States"})
	record := plugins.RawRecord{SourcePluginID: "wspr", SourceInstanceID: "test", OriginalID: "five", Domain: "hamradio", ObservedUTC: time.Now(), Payload: payload}
	messages, err := New().Normalize(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	link := messages[len(messages)-1]
	if link.Properties["display.fromRegion"] != "Switzerland" || link.Properties["display.toRegion"] != "United States" {
		t.Fatalf("plain region fallback did not apply: %#v", link.Properties)
	}
	if _, present := link.Properties["txDxcc"]; present {
		t.Fatalf("no dxcc code was supplied, none must be reported: %#v", link.Properties)
	}
}
