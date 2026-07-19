package recording

import (
	"os"
	"path/filepath"
	"sort"
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

func TestRetentionDeletesOldestFiles(t *testing.T) {
	directory := t.TempDir()
	// 100-byte cap; each hour writes ~60 bytes, so hour three must evict
	// hour one but never the file currently being written.
	writer := New(directory, true, time.Hour).WithRetention(100)
	payload := []byte(`{"x":"0123456789012345678901234567890123456789012345"}`)
	hours := []time.Time{
		time.Date(2026, 7, 19, 1, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 19, 2, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 19, 3, 0, 0, 0, time.UTC),
	}
	for _, hour := range hours {
		if err := writer.Write(hour, payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var remaining []string
	filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			remaining = append(remaining, filepath.Base(path))
		}
		return nil
	})
	sort.Strings(remaining)
	if len(remaining) != 2 || remaining[0] != "events-20260719-02.jsonl" || remaining[1] != "events-20260719-03.jsonl" {
		t.Fatalf("expected oldest hour evicted, got %v", remaining)
	}
}

func TestRetentionZeroKeepsEverything(t *testing.T) {
	directory := t.TempDir()
	writer := New(directory, true, time.Hour)
	for hour := 1; hour <= 3; hour++ {
		if err := writer.Write(time.Date(2026, 7, 19, hour, 0, 0, 0, time.UTC), []byte(`{"x":1}`)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	count := 0
	filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return nil
	})
	if count != 3 {
		t.Fatalf("unbounded mode must keep all files, got %d", count)
	}
}
