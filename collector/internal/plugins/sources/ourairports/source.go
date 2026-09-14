// Package ourairports loads the OurAirports airport CSV (public domain /
// community database) and emits large and scheduled-medium airports as
// points. Cached on disk; not embedded (the full CSV is ~12 MB).
package ourairports

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
	defaultURL  = "https://davidmegginson.github.io/ourairports-data/airports.csv"
	pollDefault = 24 * time.Hour
	pollFloor   = 6 * time.Hour
	maxAirports = 1200
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
	if sourceConfig.Type != "aviation.ourairports" {
		return nil, fmt.Errorf("unsupported ourairports source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("ourairports poll interval below six hours (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "ourairports"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "airports.csv"), TTL: interval},
		client: &http.Client{Timeout: 90 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.ourairports" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("ourairports using disk cache", "error", err)
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
		return nil, fmt.Errorf("decode ourairports csv: %w", err)
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
	records := make([]plugins.RawRecord, 0, maxAirports)
	for _, row := range rows[1:] {
		kind := col(row, "type")
		scheduled := strings.EqualFold(col(row, "scheduled_service"), "yes")
		if kind != "large_airport" && !(kind == "medium_airport" && scheduled) {
			continue
		}
		lat, errLat := strconv.ParseFloat(col(row, "latitude_deg"), 64)
		lon, errLon := strconv.ParseFloat(col(row, "longitude_deg"), 64)
		if errLat != nil || errLon != nil {
			continue
		}
		id := col(row, "ident")
		if id == "" {
			id = col(row, "id")
		}
		if id == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "airport", "airportId": id, "ident": id,
			"name": col(row, "name"), "airportKind": kind, "iata": col(row, "iata_code"),
			"latitude": lat, "longitude": lon,
			"attribution": "OurAirports (community database)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "ourairports", SourceInstanceID: s.id, OriginalID: "airport-" + id,
			Domain: "aviation", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxAirports {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("ourairports sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("ourairports snapshot", "source", s.id, "airports", len(records))
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
