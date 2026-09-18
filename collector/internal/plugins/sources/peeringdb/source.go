// Package peeringdb polls PeeringDB facility points (guest GET) that have
// coordinates. Runtime points only — this plugin does not republish the
// catalog. Attribute PeeringDB.
package peeringdb

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultURL    = "https://www.peeringdb.com/api/fac"
	pollDefault   = 24 * time.Hour
	pollFloor     = 6 * time.Hour
	maxFacilities = 250
	pageSize      = 250
	maxPages      = 8
)

type Source struct {
	id       string
	url      string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	fetch    func(ctx context.Context, rawURL string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "geography.peeringdb" {
		return nil, fmt.Errorf("unsupported peeringdb source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("peeringdb poll interval below six hours (got %s)", interval)
	}
	url := defaultURL
	if sourceConfig.Broker != "" {
		url = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "peeringdb"))
	source := &Source{
		id: sourceConfig.ID, url: url, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "fac.json"), TTL: interval},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, rawURL, map[string]string{"Accept": "application/json"})
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "geography.peeringdb" }

type facPage struct {
	Data []facRow `json:"data"`
}

type facRow struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	City      string   `json:"city"`
	Country   string   `json:"country"`
	NetCount  int      `json:"net_count"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	if cached, _, ok := s.cache.Load(); ok {
		return s.recordsFrom(cached)
	}
	var all []facRow
	var lastBody []byte
	for page := 0; page < maxPages && len(all) < maxFacilities*2; page++ {
		if page > 0 {
			if err := pollutil.Sleep(ctx, 600*time.Millisecond); err != nil {
				return nil, err
			}
		}
		rawURL := fmt.Sprintf("%s?limit=%d&skip=%d", s.url, pageSize, page*pageSize)
		if page == 0 {
			rawURL = s.url
			if !containsQuery(s.url) {
				rawURL = fmt.Sprintf("%s?limit=%d", s.url, pageSize)
			}
		}
		body, err := s.fetch(ctx, rawURL)
		if err != nil {
			if page == 0 {
				if cached, ok := s.cache.LoadStale(); ok {
					s.logger.Warn("peeringdb using disk cache", "error", err)
					return s.recordsFrom(cached)
				}
				return nil, err
			}
			break
		}
		lastBody = body
		var pageDoc facPage
		if err := json.Unmarshal(body, &pageDoc); err != nil {
			return nil, fmt.Errorf("decode peeringdb: %w", err)
		}
		if len(pageDoc.Data) == 0 {
			break
		}
		all = append(all, pageDoc.Data...)
		if len(pageDoc.Data) < pageSize {
			break
		}
	}
	if len(lastBody) > 0 {
		if snapshot, err := json.Marshal(facPage{Data: all}); err == nil {
			_ = s.cache.Store(snapshot)
			return s.recordsFrom(snapshot)
		}
	}
	return nil, fmt.Errorf("peeringdb returned no facilities")
}

func containsQuery(rawURL string) bool {
	for _, ch := range rawURL {
		if ch == '?' {
			return true
		}
	}
	return false
}

func (s *Source) recordsFrom(body []byte) ([]plugins.RawRecord, error) {
	var doc facPage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode peeringdb: %w", err)
	}
	sort.SliceStable(doc.Data, func(i, j int) bool {
		if doc.Data[i].NetCount != doc.Data[j].NetCount {
			return doc.Data[i].NetCount > doc.Data[j].NetCount
		}
		return doc.Data[i].ID < doc.Data[j].ID
	})
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxFacilities)
	for _, row := range doc.Data {
		if row.Latitude == nil || row.Longitude == nil {
			continue
		}
		if *row.Latitude == 0 && *row.Longitude == 0 {
			continue
		}
		id := strconv.Itoa(row.ID)
		if row.ID == 0 || row.Name == "" {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "facility", "facilityId": id, "name": row.Name, "city": row.City, "country": row.Country,
			"netCount": row.NetCount, "latitude": *row.Latitude, "longitude": *row.Longitude,
			"attribution": "PeeringDB",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "peeringdb", SourceInstanceID: s.id, OriginalID: "peeringdb-" + id,
			Domain: "geography", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxFacilities {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("peeringdb sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("peeringdb snapshot", "source", s.id, "facilities", len(records))
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
