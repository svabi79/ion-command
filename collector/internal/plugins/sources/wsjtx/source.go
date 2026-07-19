package wsjtx

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/plugins/sources/pskreporter"
)

// Source listens for WSJT-X UDP reports. Every decode becomes a ham-radio
// spot: the remote transmitter was heard by the local WSJT-X station. Grids
// for non-CQ callers are remembered from earlier CQ decodes.
type Source struct {
	id         string
	listenAddr string
	logger     *slog.Logger

	mu        sync.Mutex
	dialFreq  uint64
	deCall    string
	deGrid    string
	gridCache map[string]string
	sequence  uint64
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "wsjtx.udp" {
		return nil, fmt.Errorf("unsupported WSJT-X source type %q", sourceConfig.Type)
	}
	listen := sourceConfig.Broker
	if listen == "" {
		listen = "127.0.0.1:2237"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Source{id: sourceConfig.ID, listenAddr: listen, logger: logger, gridCache: make(map[string]string)}, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "wsjtx.udp" }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	address, err := net.ResolveUDPAddr("udp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("resolve WSJT-X listen address: %w", err)
	}
	connection, err := net.ListenUDP("udp", address)
	if err != nil {
		return fmt.Errorf("listen for WSJT-X datagrams: %w", err)
	}
	defer connection.Close()
	s.logger.Info("WSJT-X UDP source listening", "address", s.listenAddr)
	go func() {
		<-ctx.Done()
		connection.Close()
	}()

	buffer := make([]byte, 8192)
	for {
		if ctx.Err() != nil {
			return nil
		}
		count, _, err := connection.ReadFromUDP(buffer)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read WSJT-X datagram: %w", err)
		}
		record, ok := s.handleDatagram(buffer[:count])
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

func (s *Source) handleDatagram(datagram []byte) (plugins.RawRecord, bool) {
	header, r, err := ParseHeader(datagram)
	if err != nil {
		return plugins.RawRecord{}, false
	}
	switch header.MessageType {
	case 1: // Status
		status, err := ParseStatus(r)
		if err != nil {
			s.logger.Warn("WSJT-X status skipped", "error", err)
			return plugins.RawRecord{}, false
		}
		s.mu.Lock()
		s.dialFreq = status.DialFrequencyHz
		if status.DECall != "" {
			s.deCall = status.DECall
		}
		if status.DEGrid != "" {
			s.deGrid = status.DEGrid
		}
		s.mu.Unlock()
		return plugins.RawRecord{}, false
	case 2: // Decode
		decode, err := ParseDecode(r)
		if err != nil {
			s.logger.Warn("WSJT-X decode skipped", "error", err)
			return plugins.RawRecord{}, false
		}
		return s.spotFromDecode(decode)
	default:
		return plugins.RawRecord{}, false
	}
}

func (s *Source) spotFromDecode(decode DecodeMessage) (plugins.RawRecord, bool) {
	callsign, grid := ExtractSenderAndGrid(decode.Text)
	if callsign == "" {
		return plugins.RawRecord{}, false
	}
	s.mu.Lock()
	if grid != "" {
		s.gridCache[callsign] = grid
	} else {
		grid = s.gridCache[callsign]
	}
	dialFreq := s.dialFreq
	deCall := s.deCall
	deGrid := s.deGrid
	s.sequence++
	sequence := s.sequence
	s.mu.Unlock()
	if grid == "" || deCall == "" || deGrid == "" {
		return plugins.RawRecord{}, false
	}

	txLat, txLon, err := pskreporter.MaidenheadToLatLon(grid)
	if err != nil {
		return plugins.RawRecord{}, false
	}
	rxLat, rxLon, err := pskreporter.MaidenheadToLatLon(deGrid)
	if err != nil {
		return plugins.RawRecord{}, false
	}
	frequency := int64(dialFreq) + int64(decode.DeltaFreqHz)
	if frequency <= 0 {
		return plugins.RawRecord{}, false
	}
	payload, err := json.Marshal(map[string]any{
		"spotId":      fmt.Sprintf("wsjtx-%d", sequence),
		"txCallsign":  callsign,
		"rxCallsign":  deCall,
		"txLongitude": txLon,
		"txLatitude":  txLat,
		"rxLongitude": rxLon,
		"rxLatitude":  rxLat,
		"frequencyHz": frequency,
		"band":        BandFromFrequencyHz(frequency),
		"mode":        decode.Mode,
		"snrDb":       decode.SNRDb,
	})
	if err != nil {
		return plugins.RawRecord{}, false
	}
	return plugins.RawRecord{
		SourcePluginID:   "wsjtx",
		SourceInstanceID: s.id,
		OriginalID:       fmt.Sprintf("wsjtx-%s-%d", s.id, sequence),
		Domain:           "hamradio",
		ObservedUTC:      time.Now().UTC(),
		Payload:          payload,
	}, true
}
