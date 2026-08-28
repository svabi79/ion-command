// Package aprsis streams raw packets from APRS-IS (aprs-is.net), the
// internet backbone of the Automatic Packet Reporting System. It only knows
// TCP framing and the APRS-IS server protocol: the login line, the
// server-side filter, and the "#"-prefixed banner/keepalive comments. It has
// no idea what a callsign or an APRS symbol is - every packet line it reads
// is handed to the aprs domain untouched, as required by this project's
// source/domain split.
//
// APRS-IS is run by volunteers on donated infrastructure. This source always
// logs in with the documented read-only passcode ("-1"), so it can never
// inject traffic onto amateur radio networks; it only observes.
package aprsis

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

// defaultAddress rotates (via DNS) across aprs2.net's member servers on the
// standard client port, which supports server-side filters.
const defaultAddress = "rotate.aprs2.net:14580"

// readOnlyPasscode is the documented APRS-IS passcode meaning "accept this
// login but treat the connection as receive-only". This collector never
// transmits onto amateur radio networks, so a real passcode is never needed.
const readOnlyPasscode = "-1"

const versionString = "ion-command-collector 0.1"

// defaultFilter bounds the feed to an illustrative region - the same example
// point used by the aviation.adsb source - plus position-bearing packet
// types (p=position, o=object, i=item), rather than subscribing to the
// entire world firehose. See buildFilter for how configuration overrides it.
const defaultFilter = "r/50.0/8.0/300 t/poi"

// defaultRangeKm is used when a source is configured with latitude/longitude
// but no explicit radiusNm.
const defaultRangeKm = 200.0

// nmToKm converts the config's RadiusNm (shared with aviation.adsb, whose
// name is nautical-miles specific) into the kilometres APRS-IS expects.
const nmToKm = 1.852

type Source struct {
	id       string
	address  string
	login    string
	filter   string
	logger   *slog.Logger
	sequence uint64
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "aprs.is" {
		return nil, fmt.Errorf("unsupported aprs.is source type %q", sourceConfig.Type)
	}
	if strings.TrimSpace(sourceConfig.Login) == "" {
		return nil, fmt.Errorf("aprs.is source requires a login callsign")
	}
	if logger == nil {
		logger = slog.Default()
	}
	address := defaultAddress
	if sourceConfig.Broker != "" {
		address = sourceConfig.Broker
	}
	return &Source{
		id:      sourceConfig.ID,
		address: address,
		login:   strings.ToUpper(strings.TrimSpace(sourceConfig.Login)),
		filter:  buildFilter(sourceConfig),
		logger:  logger,
	}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aprs.is" }

// buildFilter picks the server-side subscription filter. An explicit
// sourceConfig.Filter wins verbatim; the sentinel value "world" requests no
// filter (the full feed); a configured latitude/longitude builds a range
// filter around it (radiusNm converted to km, defaulting to defaultRangeKm);
// otherwise the shipped illustrative default applies.
func buildFilter(sourceConfig config.Source) string {
	switch {
	case sourceConfig.Filter == "world":
		return ""
	case sourceConfig.Filter != "":
		return sourceConfig.Filter
	case sourceConfig.Latitude != 0 || sourceConfig.Longitude != 0:
		radiusKm := sourceConfig.RadiusNm * nmToKm
		if radiusKm <= 0 {
			radiusKm = defaultRangeKm
		}
		// Round to avoid float noise (e.g. 100nm*1.852 == 185.20000000000002)
		// leaking into a login line sent verbatim to a third-party server.
		radiusKm = math.Round(radiusKm*10) / 10
		return fmt.Sprintf("r/%g/%g/%g", sourceConfig.Latitude, sourceConfig.Longitude, radiusKm)
	default:
		return defaultFilter
	}
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	const baseBackoff = 5 * time.Second
	const maxBackoff = 5 * time.Minute
	backoff := baseBackoff
	for ctx.Err() == nil {
		started := time.Now()
		err := s.stream(ctx, output)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			s.logger.Warn("aprs.is stream ended", "error", err)
		}
		if time.Since(started) > time.Minute {
			// The connection was up long enough to be considered healthy;
			// do not let a later blip inherit a long-escalated backoff.
			backoff = baseBackoff
		} else {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
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

	login := fmt.Sprintf("user %s pass %s vers %s", s.login, readOnlyPasscode, versionString)
	if s.filter != "" {
		login += " filter " + s.filter
	}
	if _, err := conn.Write([]byte(login + "\r\n")); err != nil {
		return fmt.Errorf("send login: %w", err)
	}
	s.logger.Info("aprs.is connecting", "address", s.address, "login", s.login, "filter", s.filter)

	reader := bufio.NewReader(conn)
	for {
		// The server sends a "#" keepalive comment roughly every 20-30s even
		// during quiet spells; a couple of minutes without any line at all
		// means the connection is dead.
		conn.SetReadDeadline(time.Now().Add(2 * time.Minute))
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			// Server banner, logresp and periodic keepalive comments: the
			// IS-protocol layer's own chatter, not an APRS packet.
			s.logger.Debug("aprs.is server comment", "line", line)
			continue
		}
		s.sequence++
		record, ok := wrapLine(line, s.id, s.sequence)
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

// wrapLine packages one raw TNC2-format packet line for the aprs domain.
// The source deliberately does not look inside the line: no callsign,
// symbol or position is ever extracted here.
func wrapLine(line, sourceID string, sequence uint64) (plugins.RawRecord, bool) {
	payload, err := json.Marshal(struct {
		Raw string `json:"raw"`
	}{Raw: line})
	if err != nil {
		return plugins.RawRecord{}, false
	}
	return plugins.RawRecord{
		SourcePluginID:   "aprsis",
		SourceInstanceID: sourceID,
		OriginalID:       fmt.Sprintf("aprsis-%s-%d", sourceID, sequence),
		Domain:           "aprs",
		ObservedUTC:      time.Now().UTC(),
		Payload:          payload,
	}, true
}
