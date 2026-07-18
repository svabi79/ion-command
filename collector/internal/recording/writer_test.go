package recording

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriterCreatesHourlyJSONL(t *testing.T) {
	directory := t.TempDir()
	writer := New(directory, true, time.Millisecond)
	observed := time.Date(2026, 7, 18, 18, 30, 0, 0, time.UTC)
	if err := writer.Write(observed, []byte(`{"messageId":"one"}`)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "2026", "07", "18", "events-20260718-18.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\"messageId\":\"one\"}\n" {
		t.Fatalf("unexpected file: %q", data)
	}
}
