// Package rbn streams CW/RTTY/digital skimmer spots from the Reverse Beacon
// Network telnet feed (telnet.reversebeacon.net:7000, data courtesy of the
// RBN project and its skimmer operators). Both endpoints are placed at their
// DXCC entity centroids via the AD1C country file - country-level accuracy
// by design, which reads correctly at globe scale.
package rbn

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/cty"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const defaultAddress = "telnet.reversebeacon.net:7000"

// DX de HA8TKS-#: 14053.00  G4EDG/P        CW     7 dB  24 WPM  CQ      1528Z
var spotPattern = regexp.MustCompile(`^DX de ([A-Z0-9/\-]+?)(?:-#)?:\s+([\d.]+)\s+([A-Z0-9/]+)\s+(\S+)\s+(-?\d+) dB\s+(?:(\d+) (?:WPM|BPS)\s+)?(\S+)?\s*(\d{4})Z`)

var bandEdges = []struct {
	band string
	low  float64
	high float64
}{
	{"160m", 1800, 2000}, {"80m", 3500, 4000}, {"60m", 5250, 5450},
	{"40m", 7000, 7300}, {"30m", 10100, 10150}, {"20m", 14000, 14350},
	{"17m", 18068, 18168}, {"15m", 21000, 21450}, {"12m", 24890, 24990},
	{"10m", 28000, 29700}, {"6m", 50000, 54000},
}

func bandForKhz(khz float64) string {
	for _, edge := range bandEdges {
		if khz >= edge.low && khz <= edge.high {
			return edge.band
		}
	}
	return ""
}

type Source struct {
	id      string
	address string
	login   string
	logger  *slog.Logger
	// sequence disambiguates same-second spots in original ids.
	sequence uint64
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "hamradio.rbn" {
		return nil, fmt.Errorf("unsupported rbn source type %q", sourceConfig.Type)
	}
	if sourceConfig.Login == "" {
		return nil, fmt.Errorf("rbn source requires a login callsign")
	}
	if logger == nil {
		logger = slog.Default()
	}
	address := defaultAddress
	if sourceConfig.Broker != "" {
		address = sourceConfig.Broker
	}
	return &Source{id: sourceConfig.ID, address: address, login: strings.ToUpper(sourceConfig.Login), logger: logger}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "hamradio.rbn" }

// ParseSpot converts one telnet line into a ham-radio raw record; both ends
// resolve through the country file, unresolvable calls are skipped.
func (s *Source) ParseSpot(line string) (plugins.RawRecord, bool) {
	match := spotPattern.FindStringSubmatch(strings.TrimSpace(line))
	if match == nil {
		return plugins.RawRecord{}, false
	}
	spotter, dx := match[1], match[3]
	khz, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return plugins.RawRecord{}, false
	}
	band := bandForKhz(khz)
	if band == "" {
		return plugins.RawRecord{}, false
	}
	snr, _ := strconv.Atoi(match[5])
	txEntity, txOk := cty.Lookup(dx)
	rxEntity, rxOk := cty.Lookup(spotter)
	if !txOk || !rxOk {
		return plugins.RawRecord{}, false
	}
	s.sequence++
	payload, err := json.Marshal(map[string]any{
		"spotId":      fmt.Sprintf("rbn-%d", s.sequence),
		"txCallsign":  dx,
		"rxCallsign":  spotter,
		"txLongitude": txEntity.Longitude,
		"txLatitude":  txEntity.Latitude,
		"rxLongitude": rxEntity.Longitude,
		"rxLatitude":  rxEntity.Latitude,
		"frequencyHz": int64(khz * 1000.0),
		"band":        band,
		"mode":        match[4],
		"snrDb":       snr,
		"txRegion":    txEntity.Name,
		"rxRegion":    rxEntity.Name,
	})
	if err != nil {
		return plugins.RawRecord{}, false
	}
	return plugins.RawRecord{
		SourcePluginID:   "rbn",
		SourceInstanceID: s.id,
		OriginalID:       fmt.Sprintf("rbn-%s-%d", s.id, s.sequence),
		Domain:           "hamradio",
		ObservedUTC:      time.Now().UTC(),
		Payload:          payload,
	}, true
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for ctx.Err() == nil {
		if err := s.stream(ctx, output); err != nil && ctx.Err() == nil {
			s.logger.Warn("rbn stream ended", "error", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
		}
	}
	return nil
}

func (s *Source) stream(ctx context.Context, output chan<- plugins.RawRecord) error {
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("dial %s: %w", s.address, err)
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	// The login prompt has no newline; answer after the first read.
	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	prompt := make([]byte, 256)
	if _, err := conn.Read(prompt); err != nil {
		return fmt.Errorf("read login prompt: %w", err)
	}
	if _, err := conn.Write([]byte(s.login + "\r\n")); err != nil {
		return fmt.Errorf("send login: %w", err)
	}
	s.logger.Info("rbn connected", "address", s.address, "login", s.login)
	reader := bufio.NewReader(conn)
	for {
		conn.SetReadDeadline(time.Now().Add(3 * time.Minute))
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		record, ok := s.ParseSpot(line)
		if !ok {
			continue
		}
		select {
		case output <- record:
		case <-ctx.Done():
			return nil
		}
	}
}
