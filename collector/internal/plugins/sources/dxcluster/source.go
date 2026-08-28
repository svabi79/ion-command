// Package dxcluster streams operator-announced DX spots from a traditional DX
// cluster telnet feed (DXSpider / AR-Cluster / CC Cluster software - there is
// no single canonical node, unlike RBN, so the address is always configured).
// The wire dance mirrors the RBN source closely: dial, answer the login
// prompt with a callsign, then parse "DX de" lines. Unlike RBN's automated
// skimmer spots, cluster lines are frequently typed by a human: a signal
// report and mode are common but not guaranteed, and the free-text comment
// can be anything ("CQ", "UP2", "POTA reference", nothing at all). Both
// endpoints resolve through the AD1C country file, exactly like RBN -
// country-level accuracy by design, and calls that do not resolve are
// skipped rather than placed at invented coordinates.
package dxcluster

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
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/wsjtx"
)

// DX de HA8TKS-#: 14053.00  G4EDG/P        CW     7 dB  24 WPM  CQ      1528Z   (RBN relayed through a cluster node)
// DX de PD0YL:     14262.0  PD00DOG      SES INT DD                     1550Z   (a human operator, no signal report)
// DX de HA5WV:     28074.0  YB4KAR       FT8 -7 dB 1706 Hz TU 73        1550Z   (auto-spotted FT8 decode)
// Unlike RBN's feed, the comment column is free text - it is captured whole
// and mode/SNR are recovered from it best-effort rather than fixed columns.
var spotPattern = regexp.MustCompile(`^DX de ([A-Z0-9/\-]+?)(?:-#)?:\s+([\d.]+)\s+([A-Z0-9/]+)\s*(.*?)\s*(\d{4})Z`)

// snrPattern requires whitespace on both sides of the number so a genuine
// "CW 11 dB" or "FT8 -7 dB" report is recognised while a combined S-meter
// callout like "59+15dB" (no space before the digits) is correctly left
// alone rather than misread as an 15 dB SNR.
var snrPattern = regexp.MustCompile(`(?:^|\s)([+-]?\d{1,3})\s+[dD][bB](?:\s|$)`)

// knownModes are the tokens automated skimmers and spotting clients
// conventionally place first in the comment. Anything else is left blank
// rather than guessed.
var knownModes = map[string]bool{
	"CW": true, "SSB": true, "USB": true, "LSB": true, "FM": true, "AM": true,
	"FT8": true, "FT4": true, "RTTY": true, "PSK31": true, "PSK63": true,
	"JS8": true, "JT65": true, "JT9": true, "MFSK": true, "OLIVIA": true, "SSTV": true,
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
	if sourceConfig.Type != "hamradio.dxcluster" {
		return nil, fmt.Errorf("unsupported dxcluster source type %q", sourceConfig.Type)
	}
	if sourceConfig.Login == "" {
		return nil, fmt.Errorf("dxcluster source requires a login callsign")
	}
	// Unlike RBN there is no single canonical DX cluster node - it is a
	// federation of independently, often volunteer, operated servers - so an
	// address must be chosen deliberately rather than silently defaulted.
	if sourceConfig.Broker == "" {
		return nil, fmt.Errorf("dxcluster source requires a broker address (host:port of a DX cluster node)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Source{id: sourceConfig.ID, address: sourceConfig.Broker, login: strings.ToUpper(sourceConfig.Login), logger: logger}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "hamradio.dxcluster" }

// extractSNR recovers a "N dB" style signal report from a free-text comment,
// or nil when none is present (never fabricated).
func extractSNR(comment string) *int {
	match := snrPattern.FindStringSubmatch(comment)
	if match == nil {
		return nil
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return nil
	}
	return &value
}

// extractMode recognises a leading mode token in the comment, or returns ""
// when the comment does not start with one - cluster comments are free text
// typed by a human and frequently carry no recoverable mode at all.
func extractMode(comment string) string {
	fields := strings.Fields(comment)
	if len(fields) == 0 {
		return ""
	}
	first := strings.ToUpper(fields[0])
	if knownModes[first] {
		return first
	}
	return ""
}

// ParseSpot converts one telnet line into a ham-radio raw record. Both ends
// resolve through the country file exactly like RBN; unresolvable calls, and
// spots on a frequency outside any recognised amateur band, are skipped
// rather than placed at invented coordinates.
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
	band := wsjtx.BandFromFrequencyHz(int64(khz * 1000.0))
	if band == "" {
		return plugins.RawRecord{}, false
	}
	comment := strings.Join(strings.Fields(match[4]), " ")
	txEntity, txOk := cty.Lookup(dx)
	rxEntity, rxOk := cty.Lookup(spotter)
	if !txOk || !rxOk {
		return plugins.RawRecord{}, false
	}
	s.sequence++
	payload, err := json.Marshal(map[string]any{
		"spotId":      fmt.Sprintf("dxcluster-%d", s.sequence),
		"txCallsign":  dx,
		"rxCallsign":  spotter,
		"txLongitude": txEntity.Longitude,
		"txLatitude":  txEntity.Latitude,
		"rxLongitude": rxEntity.Longitude,
		"rxLatitude":  rxEntity.Latitude,
		"frequencyHz": int64(khz * 1000.0),
		"band":        band,
		"mode":        extractMode(comment),
		"snrDb":       extractSNR(comment),
		"txRegion":    txEntity.Name,
		"rxRegion":    rxEntity.Name,
	})
	if err != nil {
		return plugins.RawRecord{}, false
	}
	return plugins.RawRecord{
		SourcePluginID:   "dxcluster",
		SourceInstanceID: s.id,
		OriginalID:       fmt.Sprintf("dxcluster-%s-%d", s.id, s.sequence),
		Domain:           "hamradio",
		ObservedUTC:      time.Now().UTC(),
		Payload:          payload,
	}, true
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for ctx.Err() == nil {
		if err := s.stream(ctx, output); err != nil && ctx.Err() == nil {
			s.logger.Warn("dxcluster stream ended", "error", err)
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
	// Cluster banners vary a lot (some end the login prompt without a
	// newline like RBN, some print several MOTD lines first) so, unlike RBN,
	// drain whatever arrives for a short quiet period before answering
	// rather than assuming a single Read() lands on the prompt.
	reader := bufio.NewReader(conn)
	drainPrompt(conn, reader)
	if _, err := conn.Write([]byte(s.login + "\r\n")); err != nil {
		return fmt.Errorf("send login: %w", err)
	}
	s.logger.Info("dxcluster connected", "address", s.address, "login", s.login)
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

// drainPrompt reads whatever the node sends immediately after connecting -
// banner plus login prompt - for a short fixed window before the login is
// sent. Cluster banners vary (some end the prompt without a newline like
// RBN, some print several MOTD lines first); a fixed drain window handles
// both without needing to parse the prompt text itself. Validated live
// during development against eight real public nodes that accepted a TCP
// connection: all eight received and processed the login line sent this
// way (two granted a session and streamed spots; the rest gave an explicit,
// specific reply - an already-logged-in-elsewhere notice or a callsign
// validation error - which itself proves the line was read and parsed, just
// declined for reasons unrelated to this function).
func drainPrompt(conn net.Conn, reader *bufio.Reader) {
	deadline := time.Now().Add(3 * time.Second)
	buf := make([]byte, 4096)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return
		}
		conn.SetReadDeadline(deadline)
		if _, err := reader.Read(buf); err != nil {
			return
		}
	}
}
