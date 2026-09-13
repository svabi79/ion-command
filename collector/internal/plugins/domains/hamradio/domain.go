package hamradio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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
	// SNRDb is a pointer: automated decodes (PSKReporter, RBN, WSJT-X, WSPR)
	// always report one, but a human-typed DX cluster spot frequently does
	// not. Absent stays absent rather than being fabricated as 0 dB.
	SNRDb  *int `json:"snrDb"`
	TXDxcc *int `json:"txDxcc"`
	RXDxcc *int `json:"rxDxcc"`
	// Region names resolved by the source itself (via the country file) when
	// no ADIF DXCC codes are available - used by RBN, the DX cluster and WSPR.
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

// primaryLabel joins band and mode with the frequency, omitting either that
// is unknown (a DX cluster spot frequently cannot recover a mode from its
// free-text comment) instead of rendering an empty segment.
func primaryLabel(band, mode string, frequencyHz int64) string {
	parts := make([]string, 0, 3)
	if band != "" {
		parts = append(parts, band)
	}
	if mode != "" {
		parts = append(parts, mode)
	}
	parts = append(parts, fmt.Sprintf("%.3f MHz", float64(frequencyHz)/1_000_000.0))
	return strings.Join(parts, "  //  ")
}

func New() *Domain               { return &Domain{seenEntities: make(map[string]time.Time), maxSeenEntities: 200000} }
func (d *Domain) ID() string     { return "domain.hamradio" }
func (d *Domain) Domain() string { return "hamradio" }

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(record.Payload, &kind) == nil && (kind.Kind == "last-heard" || kind.Kind == "activity") {
		return d.normalizeActivity(record)
	}
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
		"frequencyHz": raw.FrequencyHz, "band": raw.Band, "mode": raw.Mode,
		"representation": "Observed Link",
		"display.title":  "Observed Link", "display.from": raw.TXCallsign, "display.to": raw.RXCallsign,
		"display.primary": primaryLabel(raw.Band, raw.Mode, raw.FrequencyHz),
	}
	// SNR is omitted rather than fabricated as 0 dB when the source (a
	// human-typed DX cluster spot) did not carry a signal report.
	if raw.SNRDb != nil {
		relationship.Properties["snrDb"] = *raw.SNRDb
		relationship.Properties["display.secondary"] = fmt.Sprintf("SNR %+d dB", *raw.SNRDb)
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

type rawActivity struct {
	Kind          string  `json:"kind"`
	SpotID        string  `json:"spotId"`
	TXCallsign    string  `json:"txCallsign"`
	TXLongitude   float64 `json:"txLongitude"`
	TXLatitude    float64 `json:"txLatitude"`
	TXRegion      string  `json:"txRegion"`
	Talkgroup     int64   `json:"talkgroup"`
	TalkgroupName string  `json:"talkgroupName"`
	DurationS     float64 `json:"durationS"`
	Mode          string  `json:"mode"`
	TalkerAlias   string  `json:"talkerAlias"`
}

func (d *Domain) normalizeActivity(record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawActivity
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode last-heard record: %w", err)
	}
	if raw.SpotID == "" || raw.TXCallsign == "" {
		return nil, fmt.Errorf("last-heard requires id and callsign")
	}
	event := events.NewEnvelope(record.OriginalID, "hamradio", "radio.activity", events.MessageObservation,
		events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID},
		record.ObservedUTC)
	event.EntityID = "radio:activity:" + raw.TXCallsign
	event.Geometry = events.Point(raw.TXLongitude, raw.TXLatitude, 0)
	validUntil := record.ObservedUTC.Add(90 * time.Second)
	event.Time.ValidUntilUTC = &validUntil
	mode := raw.Mode
	if mode == "" {
		mode = "DMR"
	}
	title := raw.TXCallsign
	primary := mode
	if raw.Talkgroup > 0 {
		primary = fmt.Sprintf("TG %d", raw.Talkgroup)
		if raw.TalkgroupName != "" {
			primary += "  //  " + raw.TalkgroupName
		}
	}
	secondary := mode
	if raw.DurationS > 0 {
		secondary = fmt.Sprintf("%s  //  %.0fs", mode, raw.DurationS)
	}
	event.Properties = map[string]any{
		"callsign":           raw.TXCallsign,
		"mode":               mode,
		"visual.icon":        "signal",
		"visual.markerScale": 1.35,
		"visual.tint":        "0.75,0.45,0.28",
		"display.title":      title,
		"display.primary":    primary,
		"display.secondary":  secondary,
		// Country-file centroid, not a measured fix. The point layer
		// will not stack this on top of a GPS/APRS marker with the
		// same display title, and must not drag that marker here.
		"visual.centroid": true,
	}
	if raw.Talkgroup > 0 {
		event.Properties["talkgroup"] = raw.Talkgroup
	}
	if raw.TalkgroupName != "" {
		event.Properties["talkgroupName"] = raw.TalkgroupName
	}
	if raw.TXRegion != "" {
		event.Properties["display.fromRegion"] = raw.TXRegion
	}
	if raw.TalkerAlias != "" {
		event.Properties["talkerAlias"] = raw.TalkerAlias
	}
	measured := false
	event.Quality.Measured = &measured
	event.Quality.Classification = "modelled"
	return []events.Envelope{event}, nil
}

func stationEntity(messageID, entityID, callsign string, lon, lat float64, source events.SourceRef, observed time.Time) events.Envelope {
	entity := events.NewEnvelope(messageID, "hamradio", "radio.station", events.MessageEntity, source, observed)
	entity.EntityID = entityID
	entity.Geometry = events.Point(lon, lat, 0)
	entity.Properties = map[string]any{"callsign": callsign, "visual.icon": "signal"}
	return entity
}
