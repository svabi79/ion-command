// Package cems polls Copernicus EMS Rapid Mapping activations and emits
// Area records from each open activation's published extent WKT.
package cems

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/geoutil"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	listURL     = "https://rapidmapping.emergency.copernicus.eu/backend/dashboard-api/public-activations-info/?limit=40&offset=0"
	detailURL   = "https://rapidmapping.emergency.copernicus.eu/backend/dashboard-api/public-activations/?code="
	pollDefault = time.Hour
	pollFloor   = 15 * time.Minute
	maxAreas    = 12
	minVertexKm = 8.0
	maxVertices = 28
)

type Source struct {
	id        string
	listURL   string
	detailURL string
	interval  time.Duration
	cache     pollutil.FileCache
	client    *http.Client
	logger    *slog.Logger
	fetch     func(ctx context.Context, rawURL string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "humanitarian.cems" {
		return nil, fmt.Errorf("unsupported cems source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("cems poll interval below fifteen minutes (got %s)", interval)
	}
	list := listURL
	detail := detailURL
	if sourceConfig.Broker != "" {
		list = sourceConfig.Broker
	}
	if sourceConfig.Topic != "" {
		detail = sourceConfig.Topic
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "cems"))
	source := &Source{
		id: sourceConfig.ID, listURL: list, detailURL: detail, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "activations.json"), TTL: interval},
		client: &http.Client{Timeout: 45 * time.Second}, logger: logger,
	}
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, rawURL, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "humanitarian.cems" }

type listResponse struct {
	Results []struct {
		Code     string `json:"code"`
		Closed   bool   `json:"closed"`
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"results"`
}

type detailResponse struct {
	Results []struct {
		Code      string   `json:"code"`
		Name      string   `json:"name"`
		Category  string   `json:"category"`
		Closed    bool     `json:"closed"`
		Extent    string   `json:"extent"`
		Countries []string `json:"countries"`
		AOIs      []struct {
			Extent string `json:"extent"`
		} `json:"aois"`
	} `json:"results"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	listBody, err := s.fetch(ctx, s.listURL)
	if err != nil {
		if cached, ok := s.cache.LoadStale(); ok {
			s.logger.Warn("cems using disk cache", "error", err)
			listBody = cached
		} else {
			return nil, err
		}
	} else {
		_ = s.cache.Store(listBody)
	}
	var listed listResponse
	if err := json.Unmarshal(listBody, &listed); err != nil {
		return nil, fmt.Errorf("decode cems list: %w", err)
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, maxAreas)
	for _, item := range listed.Results {
		if item.Closed || strings.TrimSpace(item.Code) == "" {
			continue
		}
		detailBody, err := s.fetch(ctx, s.detailURL+item.Code)
		if err != nil {
			s.logger.Warn("cems detail failed", "code", item.Code, "error", err)
			continue
		}
		var detail detailResponse
		if json.Unmarshal(detailBody, &detail) != nil || len(detail.Results) == 0 {
			continue
		}
		row := detail.Results[0]
		wkt := row.Extent
		if len(row.AOIs) > 0 && strings.TrimSpace(row.AOIs[0].Extent) != "" {
			wkt = row.AOIs[0].Extent
		}
		polys := geoutil.DecimatePolygons(geoutil.ParseWKTPolygons(wkt), minVertexKm, maxVertices)
		if len(polys) == 0 {
			continue
		}
		country := ""
		if len(row.Countries) > 0 {
			country = row.Countries[0]
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "activation", "activationId": row.Code, "name": row.Name,
			"category": row.Category, "country": country, "polygons": polys,
			"attribution": "Copernicus EMS Rapid Mapping",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "cems", SourceInstanceID: s.id, OriginalID: "cems-" + row.Code,
			Domain: "humanitarian", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxAreas {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("cems sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("cems snapshot", "source", s.id, "areas", len(records))
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
