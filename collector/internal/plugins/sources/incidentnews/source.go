// Package incidentnews polls NOAA IncidentNews CSV of pollution/hazard
// incidents and emits Points for recent geocoded rows.
package incidentnews

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL  = "https://incidentnews.noaa.gov/raw/incidents.csv"
	pollDefault = 6 * time.Hour
	pollFloor   = time.Hour
	maxPoints   = 80
	lookBack    = 180 * 24 * time.Hour
)

type Source struct {
	id       string
	url      string
	interval time.Duration
	horizon  time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	now      func() time.Time
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "maritime.incidentnews" {
		return nil, fmt.Errorf("unsupported incidentnews source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("incidentnews poll interval below one hour (got %s)", interval)
	}
	horizon := lookBack
	if sourceConfig.LookBackHours > 0 {
		horizon = time.Duration(sourceConfig.LookBackHours) * time.Hour
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "incidentnews"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval, horizon: horizon,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "incidents.csv"), TTL: interval},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger, now: time.Now,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "maritime.incidentnews" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("incidentnews using disk cache", "error", err)
			return cached, nil
		}
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(bytes.NewReader(body))
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("decode incidentnews csv: %w", err)
	}
	index := map[string]int{}
	for i, name := range rows[0] {
		index[name] = i
	}
	col := func(row []string, name string) string {
		i, ok := index[name]
		if !ok || i >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[i])
	}
	cutoff := s.now().UTC().Add(-s.horizon)
	now := s.now().UTC()
	records := make([]plugins.RawRecord, 0, maxPoints)
	for _, row := range rows[1:] {
		lat, errLat := strconv.ParseFloat(col(row, "lat"), 64)
		lon, errLon := strconv.ParseFloat(col(row, "lon"), 64)
		if errLat != nil || errLon != nil || lon < -180 || lon > 180 || lat < -90 || lat > 90 {
			continue
		}
		opened := col(row, "open_date")
		if opened != "" {
			if when, err := time.Parse("2006-01-02", opened); err == nil && when.Before(cutoff) {
				continue
			}
		}
		id := col(row, "id")
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "incident", "incidentId": id, "name": col(row, "name"),
			"threat": col(row, "threat"), "location": col(row, "location"),
			"opened": opened, "latitude": lat, "longitude": lon,
			"attribution": "NOAA IncidentNews",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "incidentnews", SourceInstanceID: s.id, OriginalID: "incident-" + id,
			Domain: "maritime", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxPoints {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("incidentnews sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("incidentnews snapshot", "source", s.id, "points", len(records))
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		if err := pollutil.Sleep(ctx, s.interval); err != nil {
			return nil
		}
	}
}
