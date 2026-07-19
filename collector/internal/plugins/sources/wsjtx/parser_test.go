package wsjtx

import (
	"bytes"
	"encoding/binary"
	"log/slog"
	"testing"
)

func writeUint32(buffer *bytes.Buffer, value uint32) { binary.Write(buffer, binary.BigEndian, value) }
func writeUint64(buffer *bytes.Buffer, value uint64) { binary.Write(buffer, binary.BigEndian, value) }
func writeString(buffer *bytes.Buffer, value string) {
	writeUint32(buffer, uint32(len(value)))
	buffer.WriteString(value)
}

func buildStatus(dialFreq uint64, deCall, deGrid string) []byte {
	var buffer bytes.Buffer
	writeUint32(&buffer, magic)
	writeUint32(&buffer, 2) // schema
	writeUint32(&buffer, 1) // status
	writeString(&buffer, "WSJT-X")
	writeUint64(&buffer, dialFreq)
	writeString(&buffer, "FT8")
	writeString(&buffer, "")  // dx call
	writeString(&buffer, "")  // report
	writeString(&buffer, "FT8") // tx mode
	buffer.Write([]byte{0, 0, 1}) // txEnabled, transmitting, decoding
	writeUint32(&buffer, 1500)    // rx df
	writeUint32(&buffer, 1500)    // tx df
	writeString(&buffer, deCall)
	writeString(&buffer, deGrid)
	return buffer.Bytes()
}

func buildDecode(snr int32, deltaFreq uint32, mode, text string) []byte {
	var buffer bytes.Buffer
	writeUint32(&buffer, magic)
	writeUint32(&buffer, 2) // schema
	writeUint32(&buffer, 2) // decode
	writeString(&buffer, "WSJT-X")
	buffer.WriteByte(1)          // is new
	writeUint32(&buffer, 43_200_000) // QTime
	writeUint32(&buffer, uint32(snr))
	writeUint64(&buffer, 0) // delta time double
	writeUint32(&buffer, deltaFreq)
	writeString(&buffer, mode)
	writeString(&buffer, text)
	buffer.WriteByte(0) // low confidence
	buffer.WriteByte(0) // off air
	return buffer.Bytes()
}

func newTestSource(t *testing.T) *Source {
	t.Helper()
	return &Source{id: "test", gridCache: make(map[string]string), logger: slog.Default()}
}

func TestStatusThenCQDecodeProducesSpot(t *testing.T) {
	source := newTestSource(t)
	if _, ok := source.handleDatagram(buildStatus(14_074_000, "HB9TEST", "JN47")); ok {
		t.Fatal("status must not produce a record")
	}
	record, ok := source.handleDatagram(buildDecode(-7, 1210, "~", "CQ DL1ABC JO62"))
	if !ok {
		t.Fatal("expected a record from a CQ decode")
	}
	if record.Domain != "hamradio" || record.SourcePluginID != "wsjtx" {
		t.Fatalf("unexpected routing: %+v", record)
	}
	payload := string(record.Payload)
	for _, expected := range []string{`"txCallsign":"DL1ABC"`, `"rxCallsign":"HB9TEST"`, `"band":"20m"`, `"frequencyHz":14075210`} {
		if !bytes.Contains(record.Payload, []byte(expected)) {
			t.Fatalf("payload missing %s: %s", expected, payload)
		}
	}
}

func TestDecodeWithoutStatusIsSkipped(t *testing.T) {
	source := newTestSource(t)
	if _, ok := source.handleDatagram(buildDecode(-7, 1210, "~", "CQ DL1ABC JO62")); ok {
		t.Fatal("decode without local station status must be skipped")
	}
}

func TestGridCacheFillsNonCQDecodes(t *testing.T) {
	source := newTestSource(t)
	source.handleDatagram(buildStatus(7_074_000, "HB9TEST", "JN47"))
	if _, ok := source.handleDatagram(buildDecode(-3, 500, "~", "HB9TEST DL1ABC R-05")); ok {
		t.Fatal("unknown grid should skip the spot")
	}
	source.handleDatagram(buildDecode(-1, 500, "~", "CQ DL1ABC JO62"))
	record, ok := source.handleDatagram(buildDecode(-3, 500, "~", "HB9TEST DL1ABC R-05"))
	if !ok {
		t.Fatal("cached grid should allow the spot")
	}
	if !bytes.Contains(record.Payload, []byte(`"band":"40m"`)) {
		t.Fatalf("expected 40m band: %s", record.Payload)
	}
}

func TestExtractSenderAndGrid(t *testing.T) {
	cases := []struct {
		text, call, grid string
	}{
		{"CQ DL1ABC JO62", "DL1ABC", "JO62"},
		{"CQ DX JA1XYZ PM95", "JA1XYZ", "PM95"},
		{"HB9TEST DL1ABC R-05", "DL1ABC", ""},
		{"HB9TEST DL1ABC RR73", "DL1ABC", ""},
		{"HB9TEST DL1ABC 73", "DL1ABC", ""},
		{"HB9TEST DL1ABC JO62", "DL1ABC", "JO62"},
		{"HB9TEST <DL1ABC> +03", "DL1ABC", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		call, grid := ExtractSenderAndGrid(c.text)
		if call != c.call || grid != c.grid {
			t.Fatalf("%q -> %q/%q, expected %q/%q", c.text, call, grid, c.call, c.grid)
		}
	}
}

func TestBandFromFrequency(t *testing.T) {
	cases := map[int64]string{14_074_000: "20m", 7_074_000: "40m", 28_074_000: "10m", 50_313_000: "6m", 999: ""}
	for freq, band := range cases {
		if got := BandFromFrequencyHz(freq); got != band {
			t.Fatalf("%d -> %q, expected %q", freq, got, band)
		}
	}
}
