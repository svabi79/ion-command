package pskreporter

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

// mqttSpot is one message from mqtt.pskreporter.info (topic pskr/filter/v2/...).
// Field names follow the published broker schema.
type mqttSpot struct {
	Sequence   int64   `json:"sq"`
	Frequency  int64   `json:"f"`
	Mode       string  `json:"md"`
	Report     int     `json:"rp"`
	Time       float64 `json:"t"`
	TXCallsign string  `json:"sc"`
	TXLocator  string  `json:"sl"`
	RXCallsign string  `json:"rc"`
	RXLocator  string  `json:"rl"`
	Band       string  `json:"b"`
	// ADIF DXCC entity codes; pointers because the broker may omit them.
	TXDxcc *int `json:"sa"`
	RXDxcc *int `json:"ra"`
}

// normalizedSpot mirrors the ham-radio domain raw contract.
type normalizedSpot struct {
	SpotID      string  `json:"spotId"`
	TXCallsign  string  `json:"txCallsign"`
	RXCallsign  string  `json:"rxCallsign"`
	TXLongitude float64 `json:"txLongitude"`
	TXLatitude  float64 `json:"txLatitude"`
	RXLongitude float64 `json:"rxLongitude"`
	RXLatitude  float64 `json:"rxLatitude"`
	FrequencyHz int64   `json:"frequencyHz"`
	Band        string  `json:"band"`
	Mode        string  `json:"mode"`
	SNRDb       int     `json:"snrDb"`
	TXDxcc      *int    `json:"txDxcc,omitempty"`
	RXDxcc      *int    `json:"rxDxcc,omitempty"`
}

// SpotDecoder converts broker frames into ham-radio raw records. Spots that
// cannot be placed on the globe (missing or malformed locators) are skipped
// rather than fabricated at 0/0; frames that are not valid JSON are an error.
// A bounded window of recent sequence ids drops duplicates, which the broker
// redelivers around reconnects.
type SpotDecoder struct {
	// The MQTT adapter decodes frames concurrently; the dedupe window is the
	// decoder's only mutable state and must be locked.
	mu              sync.Mutex
	recentSequences map[int64]struct{}
	recentOrder     []int64
}

const dedupeWindow = 8192

func NewSpotDecoder() *SpotDecoder {
	return &SpotDecoder{recentSequences: make(map[int64]struct{}, dedupeWindow)}
}

func (d *SpotDecoder) isDuplicate(sequence int64) bool {
	if d.recentSequences == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, seen := d.recentSequences[sequence]; seen {
		return true
	}
	if len(d.recentOrder) >= dedupeWindow {
		oldest := d.recentOrder[0]
		d.recentOrder = d.recentOrder[1:]
		delete(d.recentSequences, oldest)
	}
	d.recentSequences[sequence] = struct{}{}
	d.recentOrder = append(d.recentOrder, sequence)
	return false
}

func (d *SpotDecoder) Decode(frame []byte, sourceInstanceID string) ([]plugins.RawRecord, error) {
	var spot mqttSpot
	if err := json.Unmarshal(frame, &spot); err != nil {
		return nil, fmt.Errorf("decode PSKReporter frame: %w", err)
	}
	if spot.TXCallsign == "" || spot.RXCallsign == "" || spot.Frequency <= 0 {
		return nil, nil
	}
	if d.isDuplicate(spot.Sequence) {
		return nil, nil
	}
	txLat, txLon, err := MaidenheadToLatLon(spot.TXLocator)
	if err != nil {
		return nil, nil
	}
	rxLat, rxLon, err := MaidenheadToLatLon(spot.RXLocator)
	if err != nil {
		return nil, nil
	}
	observed := time.Now().UTC()
	if spot.Time > 0 {
		observed = time.Unix(int64(spot.Time), 0).UTC()
	}
	payload, err := json.Marshal(normalizedSpot{
		SpotID:      fmt.Sprintf("pskr-%d", spot.Sequence),
		TXCallsign:  spot.TXCallsign,
		RXCallsign:  spot.RXCallsign,
		TXLongitude: txLon,
		TXLatitude:  txLat,
		RXLongitude: rxLon,
		RXLatitude:  rxLat,
		FrequencyHz: spot.Frequency,
		Band:        spot.Band,
		Mode:        spot.Mode,
		SNRDb:       spot.Report,
		TXDxcc:      spot.TXDxcc,
		RXDxcc:      spot.RXDxcc,
	})
	if err != nil {
		return nil, fmt.Errorf("encode PSKReporter spot: %w", err)
	}
	return []plugins.RawRecord{{
		SourcePluginID:   "pskreporter",
		SourceInstanceID: sourceInstanceID,
		OriginalID:       fmt.Sprintf("pskr-%s-%d", sourceInstanceID, spot.Sequence),
		Domain:           "hamradio",
		ObservedUTC:      observed,
		Payload:          payload,
	}}, nil
}

// MaidenheadToLatLon returns the centre of a 4, 6, or 8 character grid square.
func MaidenheadToLatLon(locator string) (lat, lon float64, err error) {
	loc := strings.ToUpper(strings.TrimSpace(locator))
	if len(loc) < 4 || len(loc)%2 != 0 {
		return 0, 0, fmt.Errorf("locator %q too short", locator)
	}
	if len(loc) > 8 {
		loc = loc[:8]
	}
	if loc[0] < 'A' || loc[0] > 'R' || loc[1] < 'A' || loc[1] > 'R' {
		return 0, 0, fmt.Errorf("locator %q has an invalid field", locator)
	}
	if loc[2] < '0' || loc[2] > '9' || loc[3] < '0' || loc[3] > '9' {
		return 0, 0, fmt.Errorf("locator %q has an invalid square", locator)
	}
	lon = float64(loc[0]-'A')*20.0 - 180.0 + float64(loc[2]-'0')*2.0
	lat = float64(loc[1]-'A')*10.0 - 90.0 + float64(loc[3]-'0')*1.0
	lonCell, latCell := 2.0, 1.0
	if len(loc) >= 6 {
		if loc[4] < 'A' || loc[4] > 'X' || loc[5] < 'A' || loc[5] > 'X' {
			return 0, 0, fmt.Errorf("locator %q has an invalid subsquare", locator)
		}
		lon += float64(loc[4]-'A') * (2.0 / 24.0)
		lat += float64(loc[5]-'A') * (1.0 / 24.0)
		lonCell, latCell = 2.0/24.0, 1.0/24.0
	}
	if len(loc) == 8 {
		if loc[6] < '0' || loc[6] > '9' || loc[7] < '0' || loc[7] > '9' {
			return 0, 0, fmt.Errorf("locator %q has an invalid extended square", locator)
		}
		lon += float64(loc[6]-'0') * (2.0 / 240.0)
		lat += float64(loc[7]-'0') * (1.0 / 240.0)
		lonCell, latCell = 2.0/240.0, 1.0/240.0
	}
	return lat + latCell/2.0, lon + lonCell/2.0, nil
}
