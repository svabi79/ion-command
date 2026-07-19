// Package wsjtx decodes the WSJT-X UDP reporting protocol (QDataStream
// framing) and turns local decodes into ham-radio raw records. The remote
// station is the transmitter; the receiver is the local WSJT-X instance.
package wsjtx

import (
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
)

const magic = 0xadbccbda

type Header struct {
	Schema      uint32
	MessageType uint32
	ID          string
}

type StatusMessage struct {
	DialFrequencyHz uint64
	Mode            string
	DECall          string
	DEGrid          string
}

type DecodeMessage struct {
	IsNew        bool
	SNRDb        int32
	DeltaFreqHz  uint32
	Mode         string
	Text         string
	LowConfidence bool
}

type reader struct {
	data []byte
	pos  int
}

func (r *reader) remaining() int { return len(r.data) - r.pos }

func (r *reader) uint8() (byte, error) {
	if r.remaining() < 1 {
		return 0, fmt.Errorf("short packet reading byte")
	}
	value := r.data[r.pos]
	r.pos++
	return value, nil
}

func (r *reader) uint32() (uint32, error) {
	if r.remaining() < 4 {
		return 0, fmt.Errorf("short packet reading uint32")
	}
	value := binary.BigEndian.Uint32(r.data[r.pos:])
	r.pos += 4
	return value, nil
}

func (r *reader) uint64() (uint64, error) {
	if r.remaining() < 8 {
		return 0, fmt.Errorf("short packet reading uint64")
	}
	value := binary.BigEndian.Uint64(r.data[r.pos:])
	r.pos += 8
	return value, nil
}

func (r *reader) int32() (int32, error) {
	value, err := r.uint32()
	return int32(value), err
}

func (r *reader) skip(count int) error {
	if r.remaining() < count {
		return fmt.Errorf("short packet skipping %d bytes", count)
	}
	r.pos += count
	return nil
}

// utf8 reads a QByteArray/utf8 string: int32 length (0xffffffff = null).
func (r *reader) utf8() (string, error) {
	length, err := r.uint32()
	if err != nil {
		return "", err
	}
	if length == 0xffffffff {
		return "", nil
	}
	if uint32(r.remaining()) < length {
		return "", fmt.Errorf("short packet reading string of %d bytes", length)
	}
	value := string(r.data[r.pos : r.pos+int(length)])
	r.pos += int(length)
	return value, nil
}

func ParseHeader(datagram []byte) (Header, *reader, error) {
	r := &reader{data: datagram}
	seen, err := r.uint32()
	if err != nil {
		return Header{}, nil, err
	}
	if seen != magic {
		return Header{}, nil, fmt.Errorf("not a WSJT-X datagram (magic %08x)", seen)
	}
	schema, err := r.uint32()
	if err != nil {
		return Header{}, nil, err
	}
	messageType, err := r.uint32()
	if err != nil {
		return Header{}, nil, err
	}
	id, err := r.utf8()
	if err != nil {
		return Header{}, nil, err
	}
	return Header{Schema: schema, MessageType: messageType, ID: id}, r, nil
}

func ParseStatus(r *reader) (StatusMessage, error) {
	var status StatusMessage
	var err error
	if status.DialFrequencyHz, err = r.uint64(); err != nil {
		return status, err
	}
	if status.Mode, err = r.utf8(); err != nil {
		return status, err
	}
	if _, err = r.utf8(); err != nil { // DX call
		return status, err
	}
	if _, err = r.utf8(); err != nil { // report
		return status, err
	}
	if _, err = r.utf8(); err != nil { // TX mode
		return status, err
	}
	if err = r.skip(3); err != nil { // txEnabled, transmitting, decoding
		return status, err
	}
	if _, err = r.uint32(); err != nil { // RX DF
		return status, err
	}
	if _, err = r.uint32(); err != nil { // TX DF
		return status, err
	}
	if status.DECall, err = r.utf8(); err != nil {
		return status, err
	}
	if status.DEGrid, err = r.utf8(); err != nil {
		return status, err
	}
	return status, nil
}

func ParseDecode(r *reader) (DecodeMessage, error) {
	var decode DecodeMessage
	newFlag, err := r.uint8()
	if err != nil {
		return decode, err
	}
	decode.IsNew = newFlag != 0
	if _, err = r.uint32(); err != nil { // QTime ms since midnight
		return decode, err
	}
	if decode.SNRDb, err = r.int32(); err != nil {
		return decode, err
	}
	if err = r.skip(8); err != nil { // delta time (double)
		return decode, err
	}
	if decode.DeltaFreqHz, err = r.uint32(); err != nil {
		return decode, err
	}
	if decode.Mode, err = r.utf8(); err != nil {
		return decode, err
	}
	if decode.Text, err = r.utf8(); err != nil {
		return decode, err
	}
	lowConfidence, err := r.uint8()
	if err != nil {
		return decode, err
	}
	decode.LowConfidence = lowConfidence != 0
	return decode, nil
}

var callsignPattern = regexp.MustCompile(`^[A-Z0-9]{1,3}[0-9][A-Z0-9]*[A-Z](/[A-Z0-9]+)?$`)
var gridPattern = regexp.MustCompile(`^[A-R]{2}[0-9]{2}$`)

// ExtractSenderAndGrid pulls the transmitting callsign (and its grid when the
// message carries one) out of a standard FT8/FT4 exchange.
func ExtractSenderAndGrid(text string) (callsign, grid string) {
	tokens := strings.Fields(strings.ToUpper(strings.TrimSpace(text)))
	// Drop trailing report/acknowledgement tokens (R-05, +03, RRR, RR73, 73).
	// RR73 must be tested before the grid pattern: it is intentionally shaped
	// like a Maidenhead square that no station transmits from.
	for len(tokens) > 0 {
		last := tokens[len(tokens)-1]
		if last == "RRR" || last == "RR73" || last == "73" || strings.HasPrefix(last, "R-") || strings.HasPrefix(last, "R+") ||
			strings.HasPrefix(last, "-") || strings.HasPrefix(last, "+") {
			tokens = tokens[:len(tokens)-1]
			continue
		}
		if gridPattern.MatchString(last) {
			grid = last
			tokens = tokens[:len(tokens)-1]
		}
		break
	}
	for index := len(tokens) - 1; index >= 0; index-- {
		candidate := strings.Trim(tokens[index], "<>")
		if callsignPattern.MatchString(candidate) {
			return candidate, grid
		}
	}
	return "", grid
}

// BandFromFrequencyHz maps a frequency onto the amateur band designator used
// across the visualization palette.
func BandFromFrequencyHz(freq int64) string {
	mhz := float64(freq) / 1e6
	switch {
	case mhz >= 1.8 && mhz < 2.0:
		return "160m"
	case mhz >= 3.5 && mhz < 4.0:
		return "80m"
	case mhz >= 5.2 && mhz < 5.5:
		return "60m"
	case mhz >= 7.0 && mhz < 7.3:
		return "40m"
	case mhz >= 10.1 && mhz < 10.15:
		return "30m"
	case mhz >= 14.0 && mhz < 14.35:
		return "20m"
	case mhz >= 18.068 && mhz < 18.168:
		return "17m"
	case mhz >= 21.0 && mhz < 21.45:
		return "15m"
	case mhz >= 24.89 && mhz < 24.99:
		return "12m"
	case mhz >= 28.0 && mhz < 29.7:
		return "10m"
	case mhz >= 50.0 && mhz < 54.0:
		return "6m"
	case mhz >= 144.0 && mhz < 148.0:
		return "2m"
	case mhz >= 430.0 && mhz < 440.0:
		return "70cm"
	default:
		return ""
	}
}
