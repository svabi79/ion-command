package recording

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Writer struct {
	mu            sync.Mutex
	directory     string
	enabled       bool
	flushInterval time.Duration
	maxTotalBytes int64
	currentHour   string
	file          *os.File
	buffer        *bufio.Writer
	lastFlush     time.Time
}

func New(directory string, enabled bool, flushInterval time.Duration) *Writer {
	return &Writer{directory: directory, enabled: enabled, flushInterval: flushInterval}
}

// WithRetention caps the recording directory at the given size; the oldest
// hourly files are deleted on rotation until the total fits. Zero or negative
// keeps recordings unbounded.
func (w *Writer) WithRetention(maxTotalBytes int64) *Writer {
	w.maxTotalBytes = maxTotalBytes
	return w
}

func (w *Writer) Enabled() bool { return w.enabled }

func (w *Writer) Write(observed time.Time, payload []byte) error {
	if !w.enabled {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotate(observed.UTC()); err != nil {
		return err
	}
	if _, err := w.buffer.Write(payload); err != nil {
		return fmt.Errorf("write recording: %w", err)
	}
	if err := w.buffer.WriteByte('\n'); err != nil {
		return fmt.Errorf("write recording delimiter: %w", err)
	}
	if time.Since(w.lastFlush) >= w.flushInterval {
		if err := w.buffer.Flush(); err != nil {
			return fmt.Errorf("flush recording: %w", err)
		}
		w.lastFlush = time.Now()
	}
	return nil
}

func (w *Writer) rotate(observed time.Time) error {
	hour := observed.Format("20060102-15")
	if w.currentHour == hour && w.file != nil {
		return nil
	}
	if err := w.closeLocked(); err != nil {
		return err
	}
	directory := filepath.Join(w.directory, observed.Format("2006"), observed.Format("01"), observed.Format("02"))
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create recording directory: %w", err)
	}
	path := filepath.Join(directory, "events-"+hour+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open recording: %w", err)
	}
	w.file, w.buffer, w.currentHour, w.lastFlush = file, bufio.NewWriterSize(file, 256*1024), hour, time.Now()
	w.enforceRetentionLocked(path)
	return nil
}

// enforceRetentionLocked deletes the oldest recording files until the total
// size fits the configured cap. The live PSKReporter firehose writes about
// 3 GB per hour, which fills a disk within days if left unbounded. The file
// currently being written is never deleted. Runs at most once per rotation;
// failures only log to stderr because recording must never stall the
// pipeline.
func (w *Writer) enforceRetentionLocked(activePath string) {
	if w.maxTotalBytes <= 0 {
		return
	}
	type recordingFile struct {
		path string
		size int64
	}
	var files []recordingFile
	var total int64
	filepath.Walk(w.directory, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		files = append(files, recordingFile{path: path, size: info.Size()})
		total += info.Size()
		return nil
	})
	if total <= w.maxTotalBytes {
		return
	}
	// Hourly file names sort chronologically (events-YYYYMMDD-HH.jsonl).
	sort.Slice(files, func(i, j int) bool { return filepath.Base(files[i].path) < filepath.Base(files[j].path) })
	activeAbs, _ := filepath.Abs(activePath)
	for _, candidate := range files {
		if total <= w.maxTotalBytes {
			break
		}
		candidateAbs, _ := filepath.Abs(candidate.path)
		if candidateAbs == activeAbs {
			continue
		}
		if err := os.Remove(candidate.path); err != nil {
			fmt.Fprintf(os.Stderr, "recording retention could not remove %s: %v\n", candidate.path, err)
			continue
		}
		total -= candidate.size
	}
}

func (w *Writer) Close() error { w.mu.Lock(); defer w.mu.Unlock(); return w.closeLocked() }
func (w *Writer) closeLocked() error {
	var first error
	if w.buffer != nil {
		if err := w.buffer.Flush(); err != nil {
			first = err
		}
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil && first == nil {
			first = err
		}
	}
	w.buffer, w.file, w.currentHour = nil, nil, ""
	return first
}
