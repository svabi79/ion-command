package blitzortung

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// testdata/frame_live.txt is a raw frame captured from ws1.blitzortung.org on
// 2026-07-19 (LZW-compressed, one strike over Romania with a station list).
func TestParseLiveFrame(t *testing.T) {
	frame, err := os.ReadFile("testdata/frame_live.txt")
	if err != nil {
		t.Fatal(err)
	}
	record, ok, err := ParseStrike(frame, "test")
	if err != nil || !ok {
		t.Fatalf("live frame rejected: ok=%v err=%v", ok, err)
	}
	if record.Domain != "weather" || record.SourcePluginID != "blitzortung" {
		t.Fatalf("unexpected routing: %+v", record)
	}
	var payload map[string]any
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	lat := payload["latitude"].(float64)
	lon := payload["longitude"].(float64)
	if lat < 44.0 || lat > 44.5 || lon < 26.5 || lon > 27.0 {
		t.Fatalf("strike position off (expected Romania): %v/%v", lat, lon)
	}
	if payload["stationCount"].(float64) < 5 {
		t.Fatalf("station list not carried: %v", payload["stationCount"])
	}
	if !strings.HasPrefix(payload["strikeId"].(string), "bo-") {
		t.Fatalf("unexpected strike id: %v", payload["strikeId"])
	}
	if record.ObservedUTC.Year() != 2026 {
		t.Fatalf("unexpected observed time %v", record.ObservedUTC)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, ok, err := ParseStrike([]byte("garbage that is not a frame"), "test"); ok || err == nil {
		t.Fatalf("garbage must be rejected: ok=%v err=%v", ok, err)
	}
}

func TestParseSkipsNullIsland(t *testing.T) {
	frame := []byte(`{"time":1784471272232170500,"lat":0,"lon":0,"sig":[]}`)
	if _, ok, err := ParseStrike(frame, "test"); ok || err != nil {
		t.Fatalf("null island must be skipped: ok=%v err=%v", ok, err)
	}
}
