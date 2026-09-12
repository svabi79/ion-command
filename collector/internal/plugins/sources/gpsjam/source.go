// Package gpsjam polls the daily gpsjam.org H3-resolution-4 ADS-B
// navigation-accuracy grid (https://gpsjam.org). Hex centres are decoded
// locally; the provider updates once a day after midnight UTC.
package gpsjam

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/geo"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	manifestURL = "https://gpsjam.org/data/manifest.csv"
	csvBase     = "https://gpsjam.org/data"
	pollDefault = 24 * time.Hour
	pollFloor   = time.Hour
	minAircraft = 3
	maxHexes    = 400
)

type Source struct {
	id           string
	manifestURL  string
	csvBase      string
	interval     time.Duration
	client       *http.Client
	logger       *slog.Logger
	fetch        func(ctx context.Context, url string) ([]byte, error)
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "aviation.gpsjam" {
		return nil, fmt.Errorf("unsupported gpsjam source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("gpsjam poll interval below one hour (got %s)", interval)
	}
	source := &Source{
		id:          sourceConfig.ID,
		manifestURL: manifestURL,
		csvBase:     csvBase,
		interval:    interval,
		client:      &http.Client{Timeout: 60 * time.Second},
		logger:      logger,
	}
	if sourceConfig.Broker != "" {
		source.csvBase = strings.TrimSuffix(sourceConfig.Broker, "/")
	}
	if sourceConfig.Topic != "" {
		source.manifestURL = sourceConfig.Topic
	}
	source.fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		return pollutil.Get(ctx, source.client, rawURL, nil)
	}
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.gpsjam" }

func latestDate(manifest []byte) (string, error) {
	lines := strings.Split(strings.TrimSpace(string(manifest)), "\n")
	date := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) >= 10 && line[4] == '-' && line[7] == '-' {
			if line > date {
				date = line[:10]
			}
		}
	}
	if date == "" {
		return "", fmt.Errorf("gpsjam manifest had no date")
	}
	return date, nil
}

func classify(pct float64) string {
	switch {
	case pct > 10:
		return "high"
	case pct >= 2:
		return "medium"
	default:
		return "low"
	}
}

func parseHexes(csvBody []byte) []map[string]any {
	reader := csv.NewReader(bytes.NewReader(csvBody))
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil
	}
	out := make([]map[string]any, 0, 64)
	for _, row := range rows[1:] {
		if len(row) < 3 {
			continue
		}
		hex := strings.TrimSpace(row[0])
		good := atoi(row[1])
		bad := atoi(row[2])
		total := good + bad
		if total < minAircraft {
			continue
		}
		pct := 100 * float64(bad) / float64(total)
		level := classify(pct)
		if level == "low" {
			continue
		}
		index, ok := geo.ParseH3(hex)
		if !ok {
			continue
		}
		lat, lon, ok := geo.CellToLatLng(index)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"kind":           "interference",
			"h3":             hex,
			"latitude":       lat,
			"longitude":      lon,
			"level":          level,
			"pct":            mathRound(pct),
			"badAircraft":    bad,
			"totalAircraft":  total,
		})
		if len(out) >= maxHexes {
			break
		}
	}
	return out
}

func atoi(text string) int {
	n := 0
	for _, r := range strings.TrimSpace(text) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func mathRound(value float64) float64 {
	return float64(int(value*10+0.5)) / 10
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	manifest, err := s.fetch(ctx, s.manifestURL)
	if err != nil {
		return nil, err
	}
	date, err := latestDate(manifest)
	if err != nil {
		return nil, err
	}
	body, err := s.fetch(ctx, s.csvBase+"/"+date+"-h3_4.csv")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	hexes := parseHexes(body)
	records := make([]plugins.RawRecord, 0, len(hexes))
	for _, hex := range hexes {
		payload, err := json.Marshal(hex)
		if err != nil {
			continue
		}
		h3, _ := hex["h3"].(string)
		records = append(records, plugins.RawRecord{
			SourcePluginID: "gpsjam", SourceInstanceID: s.id, OriginalID: "gpsjam-" + date + "-" + h3,
			Domain: "aviation", ObservedUTC: now, Payload: payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("gpsjam sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("gpsjam snapshot", "source", s.id, "hexes", len(records))
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.interval):
		}
	}
}
