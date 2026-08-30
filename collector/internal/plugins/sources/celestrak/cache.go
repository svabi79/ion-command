package celestrak

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// Where the last good TLE set is kept. Relative to the working directory,
	// alongside the recordings, so an operator can see and delete it.
	tleCacheDir = "data/tle-cache"
	// How old a cached set may be before it is refused. SGP4 degrades as the
	// elements age: a day or two costs little, a fortnight puts a low-orbit
	// satellite kilometres and minutes out. Predicting passes from elements
	// that stale would be worse than admitting there are none.
	tleCacheMaxAge = 14 * 24 * time.Hour
)

func (s *Source) cachePath() string {
	// One file per source instance, so two differently-configured sources
	// cannot overwrite each other's set.
	return filepath.Join(tleCacheDir, fmt.Sprintf("%s.tle", s.id))
}

// storeTLECache writes the set that was just fetched successfully.
func (s *Source) storeTLECache(body []byte) {
	path := s.cachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.logger.Warn("celestrak TLE cache directory unavailable", "error", err.Error())
		return
	}
	// Write-then-rename: a crash midway leaves the previous good set intact
	// rather than a truncated file that parses to nothing.
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o644); err != nil {
		s.logger.Warn("celestrak TLE cache write failed", "error", err.Error())
		return
	}
	if err := os.Rename(temporary, path); err != nil {
		s.logger.Warn("celestrak TLE cache rename failed", "error", err.Error())
		_ = os.Remove(temporary)
	}
}

// loadTLECache returns the last good set and its age, or reports that there is
// nothing usable. Age is returned so the caller can say how stale the answer
// it is about to give really is.
func (s *Source) loadTLECache() (body []byte, age time.Duration, ok bool) {
	path := s.cachePath()
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, false
	}
	age = s.now().Sub(info.ModTime())
	if age > tleCacheMaxAge {
		s.logger.Warn("celestrak TLE cache too old to use",
			"ageHours", int(age.Hours()), "maxAgeHours", int(tleCacheMaxAge.Hours()))
		return nil, age, false
	}
	body, err = os.ReadFile(path)
	if err != nil || len(body) == 0 {
		return nil, age, false
	}
	return body, age, true
}
