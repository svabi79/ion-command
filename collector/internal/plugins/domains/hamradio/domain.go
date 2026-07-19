package hamradio

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/events"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Domain struct {
	mu              sync.Mutex
	seenEntities    map[string]time.Time
	maxSeenEntities int
}

type rawSpot struct {
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
	TXDxcc      *int    `json:"txDxcc"`
	RXDxcc      *int    `json:"rxDxcc"`
	// Region names resolved by the source itself (RBN via the country file)
	// when no ADIF DXCC codes are available.
	TXRegion string `json:"txRegion"`
	RXRegion string `json:"rxRegion"`
}

// regionName resolves an ADIF DXCC entity code to a display name. Code 0 is
// the explicit "not within any DXCC entity" marker and stays unnamed.
func regionName(code *int) string {
	if code == nil || *code == 0 {
		return ""
	}
	if name, ok := dxccEntityNames[*code]; ok {
		return name
	}
	return fmt.Sprintf("DXCC %d", *code)
}

func New() *Domain               { return &Domain{seenEntities: make(map[string]time.Time), maxSeenEntities: 200000} }
func (d *Domain) ID() string     { return "domain.hamradio" }
func (d *Domain) Domain() string { return "hamradio" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawSpot
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode ham-radio record: %w", err)
	}
	if raw.SpotID == "" || raw.TXCallsign == "" || raw.RXCallsign == "" || raw.FrequencyHz <= 0 {
		return nil, fmt.Errorf("spot requires id, endpoint callsigns, and frequency")
	}
	source := events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}
	txID := "hamradio:station:" + raw.TXCallsign
	rxID := "hamradio:receiver:" + raw.RXCallsign
	result := make([]events.Envelope, 0, 3)

	txSeen := d.markSeen(txID, record.ObservedUTC)
	rxSeen := d.markSeen(rxID, record.ObservedUTC)

	if !txSeen {
		result = append(result, stationEntity(record.OriginalID+":tx", txID, raw.TXCallsign, raw.TXLongitude, raw.TXLatitude, source, record.ObservedUTC))
	}
	if !rxSeen {
		result = append(result, stationEntity(record.OriginalID+":rx", rxID, raw.RXCallsign, raw.RXLongitude, raw.RXLatitude, source, record.ObservedUTC))
	}

	relationship := events.NewEnvelope(record.OriginalID+":link", "hamradio", "radio.reception", events.MessageRelationship, source, record.ObservedUTC)
	relationship.FromEntityID = txID
	relationship.ToEntityID = rxID
	relationship.Geometry = events.GreatCircle(raw.TXLongitude, raw.TXLatitude, raw.RXLongitude, raw.RXLatitude)
	relationship.Properties = map[string]any{
		"spotId": raw.SpotID, "txCallsign": raw.TXCallsign, "rxCallsign": raw.RXCallsign,
		"frequencyHz": raw.FrequencyHz, "band": raw.Band, "mode": raw.Mode, "snrDb": raw.SNRDb,
		"representation": "Observed Link",
		"display.title":  "Observed Link", "display.from": raw.TXCallsign, "display.to": raw.RXCallsign,
		"display.primary":   fmt.Sprintf("%s  //  %s  //  %.3f MHz", raw.Band, raw.Mode, float64(raw.FrequencyHz)/1_000_000.0),
		"display.secondary": fmt.Sprintf("SNR %+d dB", raw.SNRDb),
	}
	if region := regionName(raw.TXDxcc); region != "" {
		relationship.Properties["txDxcc"] = *raw.TXDxcc
		relationship.Properties["display.fromRegion"] = region
	} else if raw.TXRegion != "" {
		relationship.Properties["display.fromRegion"] = raw.TXRegion
	}
	if region := regionName(raw.RXDxcc); region != "" {
		relationship.Properties["rxDxcc"] = *raw.RXDxcc
		relationship.Properties["display.toRegion"] = region
	} else if raw.RXRegion != "" {
		relationship.Properties["display.toRegion"] = raw.RXRegion
	}
	measured := true
	relationship.Quality.Measured = &measured
	result = append(result, relationship)
	return result, nil
}

func (d *Domain) markSeen(entityID string, observed time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, seen := d.seenEntities[entityID]
	if !seen && len(d.seenEntities) >= d.maxSeenEntities {
		removeCount := d.maxSeenEntities / 10
		for existing := range d.seenEntities {
			delete(d.seenEntities, existing)
			removeCount--
			if removeCount <= 0 {
				break
			}
		}
	}
	d.seenEntities[entityID] = observed
	return seen
}

func stationEntity(messageID, entityID, callsign string, lon, lat float64, source events.SourceRef, observed time.Time) events.Envelope {
	entity := events.NewEnvelope(messageID, "hamradio", "radio.station", events.MessageEntity, source, observed)
	entity.EntityID = entityID
	entity.Geometry = events.Point(lon, lat, 0)
	entity.Properties = map[string]any{"callsign": callsign}
	return entity
}
