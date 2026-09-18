// Package nwis polls USGS NWIS instantaneous-value stream gauges as Points.
// Observed gauges only — not flood-model polygons. Public domain.
package nwis

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	siteBase    = "https://waterservices.usgs.gov/nwis/site/?format=rdb&siteType=ST&hasDataTypeCd=iv&siteStatus=active&parameterCd=00060&siteOutput=expanded"
	ivBase      = "https://waterservices.usgs.gov/nwis/iv/?format=json&parameterCd=00060,00065"
	pollDefault = 15 * time.Minute
	pollFloor   = 10 * time.Minute
	catalogTTL  = 7 * 24 * time.Hour
	maxGauges   = 250
	ivChunk     = 80
)

// catalogBoxes tile CONUS, Alaska, Hawaii and Puerto Rico so a single
// unbounded site query does not 400.
var catalogBoxes = []string{
	"-125.0,31.0,-102.0,49.5",
	"-102.0,25.0,-90.0,49.5",
	"-90.0,24.0,-66.0,47.5",
	"-170.0,51.0,-129.0,72.0",
	"-161.0,18.5,-154.0,22.5",
	"-67.5,17.5,-65.0,18.6",
}

type gaugeSite struct {
	ID    string
	Name  string
	Lat   float64
	Lon   float64
	Drain float64
}

type Source struct {
	id        string
	siteBase  string
	ivBase    string
	interval  time.Duration
	cache     pollutil.FileCache
	siteCache pollutil.FileCache
	client    *http.Client
	logger    *slog.Logger
	sites     []gaugeSite
	fetchSite func(ctx context.Context, bbox string) ([]byte, error)
	fetchIV   func(ctx context.Context, sites string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.nwis" {
		return nil, fmt.Errorf("unsupported nwis source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("nwis poll interval below ten minutes (got %s)", interval)
	}
	ivURL := ivBase
	if sourceConfig.Broker != "" {
		ivURL = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "nwis"))
	source := &Source{
		id: sourceConfig.ID, siteBase: siteBase, ivBase: ivURL, interval: interval,
		cache:     pollutil.FileCache{Path: filepath.Join(cacheDir, "iv.json"), TTL: interval},
		siteCache: pollutil.FileCache{Path: filepath.Join(cacheDir, "sites.rdb"), TTL: catalogTTL},
		client:    &http.Client{Timeout: 60 * time.Second}, logger: logger,
	}
	source.fetchSite = func(ctx context.Context, bbox string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, source.siteBase+"&bBox="+bbox, nil)
	}
	source.fetchIV = func(ctx context.Context, sites string) ([]byte, error) {
		sep := "?"
		if strings.Contains(source.ivBase, "?") {
			sep = "&"
		}
		return pollutil.Get(ctx, source.client, source.ivBase+sep+"sites="+sites, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.nwis" }

func (s *Source) loadCatalog(ctx context.Context) ([]gaugeSite, error) {
	if len(s.sites) > 0 {
		return s.sites, nil
	}
	if cached, _, ok := s.siteCache.Load(); ok {
		sites := parseSiteRDB(cached)
		if len(sites) > 0 {
			s.sites = rankSites(sites)
			return s.sites, nil
		}
	}
	var combined []byte
	var sites []gaugeSite
	for i, bbox := range catalogBoxes {
		if i > 0 {
			if err := pollutil.Sleep(ctx, 200*time.Millisecond); err != nil {
				return nil, err
			}
		}
		body, err := s.fetchSite(ctx, bbox)
		if err != nil {
			s.logger.Warn("nwis site catalog tile failed", "bbox", bbox, "error", err)
			continue
		}
		combined = append(combined, body...)
		combined = append(combined, '\n')
		sites = append(sites, parseSiteRDB(body)...)
	}
	if len(sites) == 0 {
		if cached, ok := s.siteCache.LoadStale(); ok {
			s.logger.Warn("nwis using stale site catalog")
			sites = parseSiteRDB(cached)
		}
	} else if len(combined) > 0 {
		_ = s.siteCache.Store(combined)
	}
	s.sites = rankSites(sites)
	return s.sites, nil
}

func rankSites(sites []gaugeSite) []gaugeSite {
	sort.SliceStable(sites, func(i, j int) bool {
		if sites[i].Drain != sites[j].Drain {
			return sites[i].Drain > sites[j].Drain
		}
		return sites[i].ID < sites[j].ID
	})
	seen := map[string]struct{}{}
	out := make([]gaugeSite, 0, maxGauges)
	for _, site := range sites {
		if site.ID == "" || site.Lat == 0 && site.Lon == 0 {
			continue
		}
		if _, ok := seen[site.ID]; ok {
			continue
		}
		seen[site.ID] = struct{}{}
		out = append(out, site)
		if len(out) >= maxGauges {
			break
		}
	}
	return out
}

func parseSiteRDB(body []byte) []gaugeSite {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 2<<20)
	var headers []string
	var sites []gaugeSite
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if headers == nil {
			headers = cols
			continue
		}
		if len(cols) > 0 && (cols[0] == "5s" || strings.HasPrefix(cols[0], "15s") || strings.Contains(cols[0], "n")) && !looksLikeSite(cols, headers) {
			continue
		}
		row := map[string]string{}
		for i, header := range headers {
			if i < len(cols) {
				row[header] = cols[i]
			}
		}
		id := strings.TrimSpace(row["site_no"])
		if id == "" {
			continue
		}
		lat, _ := strconv.ParseFloat(strings.TrimSpace(row["dec_lat_va"]), 64)
		lon, _ := strconv.ParseFloat(strings.TrimSpace(row["dec_long_va"]), 64)
		drain, _ := strconv.ParseFloat(strings.TrimSpace(row["drain_area_va"]), 64)
		sites = append(sites, gaugeSite{ID: id, Name: strings.TrimSpace(row["station_nm"]), Lat: lat, Lon: lon, Drain: drain})
	}
	return sites
}

func looksLikeSite(cols, headers []string) bool {
	if len(headers) == 0 || len(cols) == 0 {
		return false
	}
	for i, header := range headers {
		if header == "site_no" && i < len(cols) && len(cols[i]) >= 8 {
			if _, err := strconv.Atoi(cols[i]); err == nil {
				return true
			}
		}
	}
	return false
}

type ivResponse struct {
	Value struct {
		TimeSeries []struct {
			SourceInfo struct {
				SiteName string `json:"siteName"`
				SiteCode []struct {
					Value string `json:"value"`
				} `json:"siteCode"`
				GeoLocation struct {
					GeogLocation struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
					} `json:"geogLocation"`
				} `json:"geoLocation"`
			} `json:"sourceInfo"`
			Variable struct {
				VariableCode []struct {
					Value string `json:"value"`
				} `json:"variableCode"`
			} `json:"variable"`
			Values []struct {
				Value []struct {
					Value string `json:"value"`
				} `json:"value"`
			} `json:"values"`
		} `json:"timeSeries"`
	} `json:"value"`
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	sites, err := s.loadCatalog(ctx)
	if err != nil {
		return nil, err
	}
	if len(sites) == 0 {
		return nil, fmt.Errorf("nwis site catalog empty")
	}
	byID := map[string]gaugeSite{}
	ids := make([]string, 0, len(sites))
	for _, site := range sites {
		byID[site.ID] = site
		ids = append(ids, site.ID)
	}
	type agg struct {
		site         gaugeSite
		flow, height float64
	}
	merged := map[string]*agg{}
	for start := 0; start < len(ids); start += ivChunk {
		end := start + ivChunk
		if end > len(ids) {
			end = len(ids)
		}
		if start > 0 {
			if err := pollutil.Sleep(ctx, 200*time.Millisecond); err != nil {
				return nil, err
			}
		}
		body, err := s.fetchIV(ctx, strings.Join(ids[start:end], ","))
		if err != nil {
			if start == 0 {
				if cached, ok := s.cache.LoadStale(); ok {
					s.logger.Warn("nwis using disk cache", "error", err)
					body = cached
				} else {
					return nil, err
				}
			} else {
				s.logger.Warn("nwis iv chunk failed", "error", err)
				continue
			}
		} else if start == 0 {
			_ = s.cache.Store(body)
		}
		var parsed ivResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode nwis iv: %w", err)
		}
		for _, series := range parsed.Value.TimeSeries {
			id := ""
			if len(series.SourceInfo.SiteCode) > 0 {
				id = series.SourceInfo.SiteCode[0].Value
			}
			if id == "" {
				continue
			}
			site, ok := byID[id]
			if !ok {
				site = gaugeSite{ID: id, Name: series.SourceInfo.SiteName,
					Lat: series.SourceInfo.GeoLocation.GeogLocation.Latitude,
					Lon: series.SourceInfo.GeoLocation.GeogLocation.Longitude}
			}
			if site.Lat == 0 && site.Lon == 0 {
				site.Lat = series.SourceInfo.GeoLocation.GeogLocation.Latitude
				site.Lon = series.SourceInfo.GeoLocation.GeogLocation.Longitude
			}
			if site.Name == "" {
				site.Name = series.SourceInfo.SiteName
			}
			row, ok := merged[id]
			if !ok {
				row = &agg{site: site}
				merged[id] = row
			}
			param := ""
			if len(series.Variable.VariableCode) > 0 {
				param = series.Variable.VariableCode[0].Value
			}
			value := 0.0
			if len(series.Values) > 0 && len(series.Values[0].Value) > 0 {
				value, _ = strconv.ParseFloat(series.Values[0].Value[0].Value, 64)
			}
			switch param {
			case "00060":
				row.flow = value
			case "00065":
				row.height = value
			}
		}
	}
	now := time.Now().UTC()
	records := make([]plugins.RawRecord, 0, len(merged))
	ordered := make([]*agg, 0, len(merged))
	for _, row := range merged {
		ordered = append(ordered, row)
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].site.ID < ordered[j].site.ID })
	for _, row := range ordered {
		if row.site.Lat == 0 && row.site.Lon == 0 {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind": "gauge", "stationId": row.site.ID, "name": row.site.Name,
			"streamflowCfs": row.flow, "gageHeightFt": row.height,
			"latitude": row.site.Lat, "longitude": row.site.Lon,
			"attribution": "U.S. Geological Survey (public domain)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "nwis", SourceInstanceID: s.id, OriginalID: "nwis-" + row.site.ID,
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
		if len(records) >= maxGauges {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("nwis sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("nwis snapshot", "source", s.id, "gauges", len(records))
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
