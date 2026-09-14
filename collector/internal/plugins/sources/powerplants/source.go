// Package powerplants loads the WRI Global Power Plant Database CSV
// (CC BY 4.0) and emits plants at or above 500 MW. Cached on disk.
package powerplants

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
	defaultURL  = "https://raw.githubusercontent.com/wri/global-power-plant-database/master/output_database/global_power_plant_database.csv"
	pollDefault = 168 * time.Hour
	pollFloor   = 24 * time.Hour
	minMW       = 500.0
	maxPlants   = 600
)

type Source struct {
	id       string
	url      string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "geography.powerplants" {
		return nil, fmt.Errorf("unsupported powerplants source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("powerplants poll interval below one day (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "powerplants"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "plants.csv"), TTL: interval},
		client: &http.Client{Timeout: 90 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.powerplants" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("powerplants using disk cache", "error", err)
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
	reader.ReuseRecord = true
	rows, err := reader.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("decode power plant csv: %w", err)
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
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxPlants)
	for _, row := range rows[1:] {
		mw, errMW := strconv.ParseFloat(col(row, "capacity_mw"), 64)
		if errMW != nil || mw < minMW {
			continue
		}
		lat, errLat := strconv.ParseFloat(col(row, "latitude"), 64)
		lon, errLon := strconv.ParseFloat(col(row, "longitude"), 64)
		if errLat != nil || errLon != nil {
			continue
		}
		id := col(row, "gppd_idnr")
		if id == "" {
			id = col(row, "name")
		}
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "plant", "plantId": id, "name": col(row, "name"),
			"fuel": col(row, "primary_fuel"), "capacityMw": mw,
			"country": col(row, "country_long"), "latitude": lat, "longitude": lon,
			"attribution": "Global Power Plant Database, WRI (CC BY 4.0)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "powerplants", SourceInstanceID: s.id, OriginalID: "plant-" + id,
			Domain: "geography", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxPlants {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("powerplants sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("powerplants snapshot", "source", s.id, "plants", len(records))
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
