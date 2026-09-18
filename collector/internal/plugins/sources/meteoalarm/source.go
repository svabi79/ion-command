// Package meteoalarm polls MeteoAlarm legacy Atom country feeds and emits
// weather.alert Areas from cap:polygon. Runtime fetch only — the raw feed
// is not bundled or exported. Licensed under terms equivalent to CC BY 4.0
// plus extra redistribution terms; attribute MeteoAlarm.
package meteoalarm

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/geoutil"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	feedPattern = "https://feeds.meteoalarm.org/feeds/meteoalarm-legacy-atom-%s"
	pollDefault = 10 * time.Minute
	pollFloor   = 5 * time.Minute
	maxAlerts   = 40
	minVertexKm = 15.0
	maxVertices = 32
)

// countrySlugs is the European legacy-atom index as fetched 2026-09-18
// from https://feeds.meteoalarm.org/ (includes Switzerland).
var countrySlugs = []string{
	"andorra", "austria", "belgium", "bosnia-herzegovina", "bulgaria", "croatia",
	"cyprus", "czechia", "denmark", "estonia", "finland", "france", "germany",
	"greece", "hungary", "iceland", "ireland", "israel", "italy", "latvia",
	"lithuania", "luxembourg", "malta", "moldova", "montenegro", "netherlands",
	"norway", "poland", "portugal", "republic-of-north-macedonia", "romania",
	"serbia", "slovakia", "slovenia", "spain", "sweden", "switzerland",
	"ukraine", "united-kingdom",
}

type Source struct {
	id        string
	pattern   string
	countries []string
	interval  time.Duration
	cache     pollutil.FileCache
	client    *http.Client
	logger    *slog.Logger
	now       func() time.Time
	fetch     func(ctx context.Context, slug string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "weather.meteoalarm" {
		return nil, fmt.Errorf("unsupported meteoalarm source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("meteoalarm poll interval below five minutes (got %s)", interval)
	}
	pattern := feedPattern
	countries := append([]string{}, countrySlugs...)
	if sourceConfig.Broker != "" {
		if strings.Contains(sourceConfig.Broker, "%s") {
			pattern = sourceConfig.Broker
		} else {
			pattern = sourceConfig.Broker
			countries = []string{"custom"}
		}
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "meteoalarm"))
	source := &Source{
		id: sourceConfig.ID, pattern: pattern, countries: countries, interval: interval,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, "alerts.json"), TTL: interval},
		client: &http.Client{Timeout: 40 * time.Second}, logger: logger, now: func() time.Time { return time.Now().UTC() },
	}
	source.fetch = func(ctx context.Context, slug string) ([]byte, error) {
		rawURL := source.pattern
		if strings.Contains(rawURL, "%s") {
			rawURL = fmt.Sprintf(rawURL, slug)
		}
		return pollutil.Get(ctx, source.client, rawURL, map[string]string{"Accept": "application/atom+xml, application/xml"})
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "weather.meteoalarm" }

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID       string   `xml:"id"`
	Title    string   `xml:"title"`
	Updated  string   `xml:"updated"`
	Polygon  []string `xml:"urn:oasis:names:tc:emergency:cap:1.2 polygon"`
	Severity string   `xml:"urn:oasis:names:tc:emergency:cap:1.2 severity"`
	Event    string   `xml:"urn:oasis:names:tc:emergency:cap:1.2 event"`
	AreaDesc string   `xml:"urn:oasis:names:tc:emergency:cap:1.2 areaDesc"`
	Expires  string   `xml:"urn:oasis:names:tc:emergency:cap:1.2 expires"`
	Status   string   `xml:"urn:oasis:names:tc:emergency:cap:1.2 status"`
}

type parsedAlert struct {
	id, event, severity, headline, area, expires string
	rank                                         int
	polys                                        [][][][]float64
}

func (s *Source) loadCountry(ctx context.Context, slug string) ([]byte, error) {
	body, err := s.fetch(ctx, slug)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	now := s.now()
	alerts := make([]parsedAlert, 0, maxAlerts*2)
	for i, slug := range s.countries {
		if i > 0 {
			if err := pollutil.Sleep(ctx, 120*time.Millisecond); err != nil {
				return nil, err
			}
		}
		body, err := s.loadCountry(ctx, slug)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			s.logger.Warn("meteoalarm country failed", "country", slug, "error", err)
			continue
		}
		var feed atomFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			s.logger.Warn("meteoalarm decode failed", "country", slug, "error", err)
			continue
		}
		for _, entry := range feed.Entries {
			alert, ok := parseEntry(entry, now)
			if ok {
				alerts = append(alerts, alert)
			}
		}
	}
	sort.SliceStable(alerts, func(i, j int) bool {
		if alerts[i].rank != alerts[j].rank {
			return alerts[i].rank > alerts[j].rank
		}
		return alerts[i].id < alerts[j].id
	})
	if len(alerts) > maxAlerts {
		alerts = alerts[:maxAlerts]
	}
	records := make([]plugins.RawRecord, 0, len(alerts))
	for _, alert := range alerts {
		payload, err := json.Marshal(map[string]any{
			"kind":        "alert",
			"alertId":     alert.id,
			"event":       alert.event,
			"severity":    alert.severity,
			"headline":    alert.headline,
			"area":        alert.area,
			"expires":     alert.expires,
			"polygons":    alert.polys,
			"provider":    "meteoalarm",
			"attribution": "MeteoAlarm (CC BY 4.0 plus extra redistribution terms; runtime fetch only)",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "meteoalarm", SourceInstanceID: s.id, OriginalID: "meteoalarm-" + alert.id,
			Domain: "weather", ObservedUTC: now, Payload: payload,
		})
	}
	if snapshot, err := json.Marshal(map[string]any{"count": len(records), "updated": now.Format(time.RFC3339)}); err == nil {
		_ = s.cache.Store(snapshot)
	}
	return records, nil
}

func parseEntry(entry atomEntry, now time.Time) (parsedAlert, bool) {
	if strings.TrimSpace(entry.Status) != "" && !strings.EqualFold(entry.Status, "Actual") {
		return parsedAlert{}, false
	}
	expires := strings.TrimSpace(entry.Expires)
	if expires != "" {
		if when, err := time.Parse(time.RFC3339, expires); err == nil && when.Before(now) {
			return parsedAlert{}, false
		}
	}
	var polys [][][][]float64
	for _, raw := range entry.Polygon {
		ring := parseCapPolygon(raw)
		if len(ring) == 0 {
			continue
		}
		reduced := geoutil.DecimatePolygons([][][][]float64{{ring}}, minVertexKm, maxVertices)
		if len(reduced) > 0 {
			polys = append(polys, reduced...)
		}
	}
	if len(polys) == 0 {
		return parsedAlert{}, false
	}
	id := strings.TrimSpace(html.UnescapeString(entry.ID))
	if id == "" {
		return parsedAlert{}, false
	}
	if idx := strings.LastIndex(id, "/"); idx >= 0 && idx < len(id)-1 {
		id = strings.SplitN(id[idx+1:], "?", 2)[0]
	}
	eventName := strings.TrimSpace(entry.Event)
	if eventName == "" {
		eventName = strings.TrimSpace(entry.Title)
	}
	severity := strings.TrimSpace(entry.Severity)
	return parsedAlert{
		id: id, event: eventName, severity: severity, headline: strings.TrimSpace(entry.Title),
		area: strings.TrimSpace(entry.AreaDesc), expires: expires, rank: severityRank(severity), polys: polys,
	}, true
}

func parseCapPolygon(raw string) [][]float64 {
	fields := strings.Fields(strings.TrimSpace(raw))
	ring := make([][]float64, 0, len(fields))
	for _, field := range fields {
		parts := strings.Split(field, ",")
		if len(parts) < 2 {
			continue
		}
		lat, errLat := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lon, errLon := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errLat != nil || errLon != nil {
			continue
		}
		ring = append(ring, []float64{lon, lat})
	}
	if len(ring) < 3 {
		return nil
	}
	if ring[0][0] != ring[len(ring)-1][0] || ring[0][1] != ring[len(ring)-1][1] {
		ring = append(ring, []float64{ring[0][0], ring[0][1]})
	}
	return ring
}

func severityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "extreme":
		return 4
	case "severe":
		return 3
	case "moderate":
		return 2
	case "minor":
		return 1
	default:
		return 0
	}
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("meteoalarm sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("meteoalarm snapshot", "source", s.id, "alerts", len(records))
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
