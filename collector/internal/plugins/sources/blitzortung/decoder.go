// Package blitzortung streams live lightning strikes from the Blitzortung.org
// community network (data courtesy of Blitzortung.org and its station
// operators; non-commercial use). Frames arrive LZW-compressed with the
// project's own dictionary scheme and decode to one JSON strike each.
package blitzortung

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

// DecodeFrame reverses the blitzortung web encoder: plain text seeds the
// dictionary and code points >= 256 reference previously seen sequences.
func DecodeFrame(frame string) string {
	runes := []rune(frame)
	if len(runes) == 0 {
		return ""
	}
	dictionary := map[int]string{}
	current := string(runes[0])
	previous := current
	var builder strings.Builder
	builder.WriteString(current)
	next := 256
	for index := 1; index < len(runes); index++ {
		code := int(runes[index])
		var entry string
		switch {
		case code < 256:
			entry = string(runes[index])
		default:
			if known, ok := dictionary[code]; ok {
				entry = known
			} else {
				entry = previous + current
			}
		}
		builder.WriteString(entry)
		current = string([]rune(entry)[0])
		dictionary[next] = previous + current
		next++
		previous = entry
	}
	return builder.String()
}

type strikeFrame struct {
	TimeNs    int64   `json:"time"`
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	MCG       int     `json:"mcg"`
	Signals   []struct {
		Station int `json:"sta"`
	} `json:"sig"`
}

// ParseStrike converts one raw websocket frame into a weather-domain raw
// record. Frames without a plausible position or time are skipped.
func ParseStrike(frame []byte, sourceInstanceID string) (plugins.RawRecord, bool, error) {
	decoded := DecodeFrame(string(frame))
	var strike strikeFrame
	if err := json.Unmarshal([]byte(decoded), &strike); err != nil {
		return plugins.RawRecord{}, false, fmt.Errorf("decode strike frame: %w", err)
	}
	if strike.TimeNs <= 0 || (strike.Latitude == 0 && strike.Longitude == 0) {
		return plugins.RawRecord{}, false, nil
	}
	if strike.Latitude < -90 || strike.Latitude > 90 || strike.Longitude < -180 || strike.Longitude > 180 {
		return plugins.RawRecord{}, false, nil
	}
	observed := time.Unix(0, strike.TimeNs).UTC()
	payload, err := json.Marshal(map[string]any{
		"strikeId":      fmt.Sprintf("bo-%d", strike.TimeNs),
		"longitude":     strike.Longitude,
		"latitude":      strike.Latitude,
		"peakCurrentKa": 0.0,
		"stationCount":  len(strike.Signals),
	})
	if err != nil {
		return plugins.RawRecord{}, false, err
	}
	return plugins.RawRecord{
		SourcePluginID:   "blitzortung",
		SourceInstanceID: sourceInstanceID,
		OriginalID:       fmt.Sprintf("bo-%s-%d", sourceInstanceID, strike.TimeNs),
		Domain:           "weather",
		ObservedUTC:      observed,
		Payload:          payload,
	}, true, nil
}
