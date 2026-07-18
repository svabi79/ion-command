package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
)

type Range struct {
	Available bool       `json:"available"`
	FromUTC   *time.Time `json:"fromUtc,omitempty"`
	ToUTC     *time.Time `json:"toUtc,omitempty"`
	FileCount int        `json:"fileCount"`
}

func AvailableRange(directory string) (Range, error) {
	files, err := recordingFiles(directory)
	if err != nil {
		return Range{}, err
	}
	result := Range{FileCount: len(files)}
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			return Range{}, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var envelope events.Envelope
			if json.Unmarshal(scanner.Bytes(), &envelope) != nil {
				continue
			}
			timestamp := envelope.Time.ObservedUTC
			if result.FromUTC == nil || timestamp.Before(*result.FromUTC) {
				value := timestamp
				result.FromUTC = &value
			}
			if result.ToUTC == nil || timestamp.After(*result.ToUTC) {
				value := timestamp
				result.ToUTC = &value
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return Range{}, scanErr
		}
		if closeErr != nil {
			return Range{}, closeErr
		}
	}
	result.Available = result.FromUTC != nil
	return result, nil
}

func Stream(ctx context.Context, directory string, from, to time.Time, speed float64, emit func([]byte) error) error {
	if speed <= 0 {
		return fmt.Errorf("speed must be positive")
	}
	files, err := recordingFiles(directory)
	if err != nil {
		return err
	}
	var previous time.Time
	for _, path := range files {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			var envelope events.Envelope
			if err := json.Unmarshal(line, &envelope); err != nil {
				continue
			}
			observed := envelope.Time.ObservedUTC
			if (!from.IsZero() && observed.Before(from)) || (!to.IsZero() && observed.After(to)) {
				continue
			}
			if !previous.IsZero() {
				delay := time.Duration(float64(observed.Sub(previous)) / speed)
				if delay > 0 {
					timer := time.NewTimer(delay)
					select {
					case <-ctx.Done():
						timer.Stop()
						_ = file.Close()
						return nil
					case <-timer.C:
					}
				}
			}
			if err := emit(line); err != nil {
				_ = file.Close()
				return err
			}
			previous = observed
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func recordingFiles(directory string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".jsonl" {
			files = append(files, path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return files, nil
	}
	sort.Strings(files)
	return files, err
}
