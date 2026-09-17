// Package wpi loads the NGA World Port Index CSV (Pub 150) and emits
// Large then Medium harbor Points. Cached on disk.
package wpi

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
	defaultURL  = "https://msi.nga.mil/api/publications/download?key=16920959/SFH00000/UpdatedPub150.csv&type=view"
	pollDefault = 168 * time.Hour
	pollFloor   = 24 * time.Hour
	maxPorts    = 400
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
	if sourceConfig.Type != "maritime.wpi" {
		return nil, fmt.Errorf("unsupported wpi source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("wpi poll interval below one day (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "wpi"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "UpdatedPub150.csv"), TTL: interval},
		client: &http.Client{Timeout: 90 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context) ([]byte, error) { return pollutil.Get(ctx, source.client, source.url, nil) }
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "maritime.wpi" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return cached, nil
	}
	body, err := s.fetch(ctx)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("wpi using disk cache", "error", err)
			return cached, nil
		}
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

func sizeRank(size string) int {
	switch strings.ToLower(strings.TrimSpace(size)) {
	case "large":
		return 0
	case "medium":
		return 1
	default:
		return 9
	}
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
		return nil, fmt.Errorf("decode world port index csv: %w", err)
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
	type item struct {
		id, name, size, country, use string
		lat, lon                     float64
		rank                         int
	}
	picked := make([]item, 0)
	for _, row := range rows[1:] {
		size := col(row, "Harbor Size")
		rank := sizeRank(size)
		if rank > 1 {
			continue
		}
		lat, errLat := strconv.ParseFloat(col(row, "Latitude"), 64)
		lon, errLon := strconv.ParseFloat(col(row, "Longitude"), 64)
		if errLat != nil || errLon != nil {
			continue
		}
		id := col(row, "World Port Index Number")
		if id == "" {
			id = col(row, "Main Port Name")
		}
		if id == "" {
			continue
		}
		if n, err := strconv.ParseFloat(id, 64); err == nil {
			id = strconv.FormatInt(int64(n), 10)
		}
		picked = append(picked, item{
			id: id, name: col(row, "Main Port Name"), size: size,
			country: col(row, "Country Code"), use: col(row, "Harbor Use"),
			lat: lat, lon: lon, rank: rank,
		})
	}
	for i := 0; i < len(picked); i++ {
		best := i
		for j := i + 1; j < len(picked); j++ {
			if picked[j].rank < picked[best].rank {
				best = j
			}
		}
		picked[i], picked[best] = picked[best], picked[i]
	}
	if len(picked) > maxPorts {
		picked = picked[:maxPorts]
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(picked))
	for _, port := range picked {
		payload, err := json.Marshal(map[string]any{
			"kind": "port", "portId": port.id, "name": port.name, "size": port.size,
			"country": port.country, "harborUse": port.use,
			"latitude": port.lat, "longitude": port.lon,
			"attribution": "NGA World Port Index (Pub 150)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "wpi", SourceInstanceID: s.id, OriginalID: "wpi-" + port.id,
			Domain: "maritime", ObservedUTC: now, Payload: payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("wpi sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("wpi snapshot", "source", s.id, "ports", len(records))
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
