package pskreporter

import (
	"encoding/json"
	"math"
	"testing"
)

func TestDecodeRealFrame(t *testing.T) {
	frame := []byte(`{"sq":123456789,"f":14074123,"md":"FT8","rp":-11,"t":1784436418,"sc":"HB9ABC","sl":"JN47ka","rc":"K1ABC","rl":"FN31pr","sa":287,"ra":291,"b":"20m"}`)
	records, err := NewSpotDecoder().Decode(frame, "primary")
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record, got %d", len(records))
	}
	record := records[0]
	if record.Domain != "hamradio" || record.SourcePluginID != "pskreporter" {
		t.Fatalf("unexpected record routing: %+v", record)
	}
	if record.OriginalID != "pskr-primary-123456789" {
		t.Fatalf("unexpected original id %q", record.OriginalID)
	}
	if record.ObservedUTC.Year() != 2026 {
		t.Fatalf("unexpected observed time %v", record.ObservedUTC)
	}
	var spot normalizedSpot
	if err := json.Unmarshal(record.Payload, &spot); err != nil {
		t.Fatalf("payload decode failed: %v", err)
	}
	if spot.TXCallsign != "HB9ABC" || spot.RXCallsign != "K1ABC" || spot.Band != "20m" || spot.Mode != "FT8" || spot.SNRDb != -11 || spot.FrequencyHz != 14074123 {
		t.Fatalf("unexpected spot content: %+v", spot)
	}
	if math.Abs(spot.TXLatitude-47.0208) > 0.01 || math.Abs(spot.TXLongitude-8.8750) > 0.01 {
		t.Fatalf("JN47ka mapped to %.4f/%.4f", spot.TXLatitude, spot.TXLongitude)
	}
	if math.Abs(spot.RXLatitude-41.73) > 0.05 || math.Abs(spot.RXLongitude-(-72.70)) > 0.05 {
		t.Fatalf("FN31pr mapped to %.4f/%.4f", spot.RXLatitude, spot.RXLongitude)
	}
	if spot.TXDxcc == nil || *spot.TXDxcc != 287 || spot.RXDxcc == nil || *spot.RXDxcc != 291 {
		t.Fatalf("sa/ra not carried through: %+v %+v", spot.TXDxcc, spot.RXDxcc)
	}
}

func TestDecodeWithoutDxccStaysNil(t *testing.T) {
	frame := []byte(`{"sq":9,"f":14074123,"md":"FT8","rp":-1,"t":1784436418,"sc":"HB9ABC","sl":"JN47ka","rc":"K1ABC","rl":"FN31pr","b":"20m"}`)
	records, err := NewSpotDecoder().Decode(frame, "primary")
	if err != nil || len(records) != 1 {
		t.Fatalf("decode failed: %v (%d records)", err, len(records))
	}
	var spot normalizedSpot
	if err := json.Unmarshal(records[0].Payload, &spot); err != nil {
		t.Fatalf("payload decode failed: %v", err)
	}
	if spot.TXDxcc != nil || spot.RXDxcc != nil {
		t.Fatalf("expected nil dxcc codes, got %+v %+v", spot.TXDxcc, spot.RXDxcc)
	}
}

func TestDecodeSkipsUnmappableSpots(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"sq":1,"f":14074000,"sc":"HB9ABC","sl":"","rc":"K1ABC","rl":"FN31"}`),
		[]byte(`{"sq":2,"f":14074000,"sc":"HB9ABC","sl":"JN47","rc":"K1ABC","rl":"ZZ99"}`),
		[]byte(`{"sq":3,"f":0,"sc":"HB9ABC","sl":"JN47","rc":"K1ABC","rl":"FN31"}`),
		[]byte(`{"sq":4,"f":14074000,"sc":"","sl":"JN47","rc":"K1ABC","rl":"FN31"}`),
	}
	for i, frame := range cases {
		records, err := NewSpotDecoder().Decode(frame, "primary")
		if err != nil {
			t.Fatalf("case %d returned error: %v", i, err)
		}
		if len(records) != 0 {
			t.Fatalf("case %d produced %d records, expected skip", i, len(records))
		}
	}
}

func TestDecodeRejectsMalformedJSON(t *testing.T) {
	if _, err := NewSpotDecoder().Decode([]byte("not json"), "primary"); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestMaidenheadConversion(t *testing.T) {
	cases := []struct {
		locator  string
		lat, lon float64
	}{
		{"JN47", 47.5, 9.0},
		{"JN47ka", 47.0208, 8.8750},
		{"FN31pr", 41.7292, -72.7083},
		{"RE78ir", -41.2708, 174.7083},
		{"AA00aa", -89.9792, -179.9583},
	}
	for _, c := range cases {
		lat, lon, err := MaidenheadToLatLon(c.locator)
		if err != nil {
			t.Fatalf("%s failed: %v", c.locator, err)
		}
		if math.Abs(lat-c.lat) > 0.01 || math.Abs(lon-c.lon) > 0.01 {
			t.Fatalf("%s mapped to %.4f/%.4f, expected %.4f/%.4f", c.locator, lat, lon, c.lat, c.lon)
		}
	}
	for _, invalid := range []string{"", "J", "JN4", "ZZ99", "JN4a", "JNxx"} {
		if _, _, err := MaidenheadToLatLon(invalid); err == nil {
			t.Fatalf("expected error for %q", invalid)
		}
	}
}

func TestDecodeDropsDuplicateSequences(t *testing.T) {
	decoder := NewSpotDecoder()
	frame := []byte(`{"sq":42,"f":14074123,"md":"FT8","rp":-1,"t":1784436418,"sc":"HB9ABC","sl":"JN47ka","rc":"K1ABC","rl":"FN31pr","b":"20m"}`)
	first, err := decoder.Decode(frame, "primary")
	if err != nil || len(first) != 1 {
		t.Fatalf("first decode: %v (%d records)", err, len(first))
	}
	second, err := decoder.Decode(frame, "primary")
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("redelivered sequence must be dropped, got %d records", len(second))
	}
}
