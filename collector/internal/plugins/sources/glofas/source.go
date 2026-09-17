// Package glofas downloads GloFAS Rapid Flood Mapping shapefiles from the
// public EFAS/GloFAS download API. The product is one huge MultiPolygon;
// only the largest rings are kept and coarsened. Cap is hard.
package glofas

import (
	"archive/zip"
	"bytes"
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
	downloadBase = "https://european-flood.emergency.copernicus.eu/api/fms/download/glofas/RapidFloodMapping/"
	pollDefault  = 6 * time.Hour
	pollFloor    = time.Hour
	maxBytes     = 12 << 20
	maxAreas     = 16
	minVertexKm  = 40.0
	maxVertices  = 24
)

type Source struct {
	id       string
	baseURL  string
	interval time.Duration
	cache    pollutil.FileCache
	client   *http.Client
	logger   *slog.Logger
	now      func() time.Time
	fetch    func(ctx context.Context, rawURL string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.glofas" {
		return nil, fmt.Errorf("unsupported glofas source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("glofas poll interval below one hour (got %s)", interval)
	}
	base := downloadBase
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "glofas"))
	source := &Source{
		id: sourceConfig.ID, baseURL: base, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "rfm.zip"), TTL: interval},
		client: &http.Client{Timeout: 90 * time.Second}, logger: logger, now: time.Now,
	}
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.GetLimit(ctx, source.client, rawURL, nil, maxBytes)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.glofas" }

func (s *Source) load(ctx context.Context) ([]byte, error) {
	now := s.now().UTC()
	var lastErr error
	for _, day := range []int{0, 1, 2} {
		when := now.Add(-time.Duration(day) * 24 * time.Hour).Truncate(24 * time.Hour)
		rawURL := strings.TrimRight(s.baseURL, "/") + "/" + when.Format("2006-01-02T15:04Z")
		body, err := s.fetch(ctx, rawURL)
		if err != nil {
			lastErr = err
			continue
		}
		if len(body) > 4 && body[0] == 'P' && body[1] == 'K' {
			_ = s.cache.Store(body)
			return body, nil
		}
		lastErr = fmt.Errorf("glofas download was not a zip")
	}
	if cached, ok := s.cache.LoadStale(); ok {
		s.logger.Warn("glofas using disk cache", "error", lastErr)
		return cached, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("glofas download empty")
	}
	return nil, lastErr
}

func shpFromZip(body []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}
	for _, file := range reader.File {
		if strings.EqualFold(filepath.Ext(file.Name), ".shp") {
			opened, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer opened.Close()
			buf := bytes.NewBuffer(make([]byte, 0, file.UncompressedSize64))
			if _, err := buf.ReadFrom(opened); err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("zip contained no shapefile")
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	shp, err := shpFromZip(body)
	if err != nil {
		return nil, err
	}
	polys, err := polygonsFromSHP(shp)
	if err != nil {
		return nil, err
	}
	polys = geoutil.DecimatePolygons(geoutil.TakeLargestPolygons(polys, maxAreas), minVertexKm, maxVertices)
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(polys))
	for i, rings := range polys {
		id := fmt.Sprintf("rfm-%d", i+1)
		payload, err := json.Marshal(map[string]any{
			"kind": "flood", "areaId": id, "title": "GloFAS Rapid Flood Mapping",
			"label": "forecast flood extent", "polygons": [][][][]float64{rings},
			"provider": "glofas", "attribution": "Copernicus EMS GloFAS Rapid Flood Mapping",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "glofas", SourceInstanceID: s.id, OriginalID: "glofas-" + id,
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("glofas sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("glofas snapshot", "source", s.id, "areas", len(records))
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
