// Package maritime normalizes raw AIS records (position reports and
// static/voyage data) into canonical per-vessel Point observations. It owns
// all AIS vocabulary: the "not available" sentinel values, the split between
// genuine ship-station MMSIs and other AIS participants (base stations, aids
// to navigation, SAR aircraft, ...), the ship/cargo type and navigational
// status code tables, and the flag-state lookup. Sources feeding this domain
// return raw, uninterpreted protocol fields only.
package maritime

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

// defaultMaxVessels bounds the per-MMSI cache that joins static/voyage data
// onto the position stream and remembers each vessel's last reported fix for
// duplicate suppression. The global AIS-fitted fleet is on the order of a
// few hundred thousand craft; this comfortably covers a busy multi-region
// subscription while still being a hard, finite bound. When full, the oldest
// ~10% of entries (by natural map iteration, the same strategy the hamradio
// domain uses for its own seen-entity cache) are evicted to make room.
const defaultMaxVessels = 100000

// reaffirmFraction: even when a vessel's position fingerprint has not
// changed, it is re-emitted once this fraction of its validity window has
// elapsed, so a moored vessel that keeps confirming an unchanged position
// never silently ages off the globe between genuine position changes.
const reaffirmFraction = 0.5

type Domain struct {
	mu         sync.Mutex
	vessels    map[int64]*vesselState
	maxVessels int
}

// vesselState is the cached, merged knowledge for one MMSI: whatever
// static/voyage fields have been learned so far (upserted field by field, as
// AIS static data usually arrives split across several messages over
// several minutes) plus the last position fingerprint that was actually
// emitted, used to suppress duplicate envelopes.
type vesselState struct {
	// Static/voyage fields, upserted from "static" raw records. Zero/empty
	// means "not yet known", never "known to be zero" (AIS's own convention
	// for most of these fields, which is why a later message with unknown
	// values need never overwrite a fact this cache already has).
	name, callSign, destination string
	shipTypeCode, imoNumber     int
	etaMonth, etaDay, etaHour   int
	etaMinute                   int
	dimA, dimB, dimC, dimD      int
	draughtM                    float64

	// Last-emitted position fingerprint, for duplicate suppression.
	hasFingerprint bool
	fingerprint    positionFingerprint
	lastEmittedAt  time.Time
	touchedAt      time.Time
}

type positionFingerprint struct {
	lon, lat  int64 // degrees * 1e5 (~1.1 m), rounded
	cog       int64 // degrees, rounded; 0 when course is not available
	heading   int   // -1 when unavailable
	navStatus int   // -1 when unavailable (Class B, or not yet known)
}

func New() *Domain               { return newDomain(defaultMaxVessels) }
func (d *Domain) ID() string     { return "domain.maritime" }
func (d *Domain) Domain() string { return "maritime" }

func newDomain(bound int) *Domain {
	return &Domain{vessels: make(map[int64]*vesselState), maxVessels: bound}
}

// rawPosition mirrors the flat, uninterpreted payload the ais.aisstream
// source emits for a PositionReport or StandardClassBPositionReport.
type rawPosition struct {
	Kind             string  `json:"kind"`
	MMSI             int64   `json:"mmsi"`
	ClassB           bool    `json:"classB"`
	Longitude        float64 `json:"lon"`
	Latitude         float64 `json:"lat"`
	SogKn            float64 `json:"sogKn"`
	CogDeg           float64 `json:"cogDeg"`
	HeadingDeg       int     `json:"headingDeg"`
	NavStatus        *int    `json:"navStatus"`
	RateOfTurn       *int    `json:"rateOfTurn"`
	PositionAccuracy bool    `json:"positionAccuracy"`
	Raim             bool    `json:"raim"`
	MetaShipName     string  `json:"metaShipName"`
}

// rawStatic mirrors the flat payload for ShipStaticData / StaticDataReport.
// Any field a particular message type does not carry decodes to its zero
// value, which for AIS static data doubles as "not provided" — see the
// package doc on the source side for why that is safe here.
type rawStatic struct {
	Kind         string  `json:"kind"`
	MMSI         int64   `json:"mmsi"`
	Name         string  `json:"name"`
	CallSign     string  `json:"callSign"`
	ShipTypeCode int     `json:"shipTypeCode"`
	ImoNumber    int     `json:"imoNumber"`
	Destination  string  `json:"destination"`
	EtaMonth     int     `json:"etaMonth"`
	EtaDay       int     `json:"etaDay"`
	EtaHour      int     `json:"etaHour"`
	EtaMinute    int     `json:"etaMinute"`
	DimA         int     `json:"dimA"`
	DimB         int     `json:"dimB"`
	DimC         int     `json:"dimC"`
	DimD         int     `json:"dimD"`
	DraughtM     float64 `json:"draughtM"`
}

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var kind struct {
		Kind string `json:"kind"`
		MMSI int64  `json:"mmsi"`
	}
	if err := json.Unmarshal(record.Payload, &kind); err != nil {
		return nil, fmt.Errorf("decode ais record: %w", err)
	}
	if kind.MMSI <= 0 {
		return nil, fmt.Errorf("ais record requires a positive mmsi")
	}
	switch kind.Kind {
	case "static":
		var raw rawStatic
		if err := json.Unmarshal(record.Payload, &raw); err != nil {
			return nil, fmt.Errorf("decode ais static record: %w", err)
		}
		d.mergeStatic(raw)
		return nil, nil
	case "position":
		var raw rawPosition
		if err := json.Unmarshal(record.Payload, &raw); err != nil {
			return nil, fmt.Errorf("decode ais position record: %w", err)
		}
		return d.normalizePosition(record, raw), nil
	default:
		return nil, fmt.Errorf("ais record has unknown kind %q", kind.Kind)
	}
}

func (d *Domain) mergeStatic(raw rawStatic) {
	d.mu.Lock()
	defer d.mu.Unlock()
	state := d.getOrCreateLocked(raw.MMSI)
	if raw.Name != "" {
		state.name = raw.Name
	}
	if raw.CallSign != "" {
		state.callSign = raw.CallSign
	}
	if raw.Destination != "" {
		state.destination = raw.Destination
	}
	if raw.ShipTypeCode != 0 {
		state.shipTypeCode = raw.ShipTypeCode
	}
	if raw.ImoNumber != 0 {
		state.imoNumber = raw.ImoNumber
	}
	// The ETA sub-fields are only meaningful together: a day-of-month without
	// a month is not a date. Month is the anchor field per ITU-R M.1371.
	if raw.EtaMonth != 0 {
		state.etaMonth, state.etaDay, state.etaHour, state.etaMinute = raw.EtaMonth, raw.EtaDay, raw.EtaHour, raw.EtaMinute
	}
	if raw.DimA != 0 || raw.DimB != 0 || raw.DimC != 0 || raw.DimD != 0 {
		state.dimA, state.dimB, state.dimC, state.dimD = raw.DimA, raw.DimB, raw.DimC, raw.DimD
	}
	if raw.DraughtM != 0 {
		state.draughtM = raw.DraughtM
	}
	state.touchedAt = time.Now().UTC()
}

// getOrCreateLocked returns the cache entry for mmsi, evicting the oldest
// slice of entries first if the cache is at capacity. Callers must hold d.mu.
func (d *Domain) getOrCreateLocked(mmsi int64) *vesselState {
	if state, ok := d.vessels[mmsi]; ok {
		return state
	}
	if len(d.vessels) >= d.maxVessels {
		removeCount := d.maxVessels / 10
		if removeCount < 1 {
			removeCount = 1
		}
		for existing := range d.vessels {
			delete(d.vessels, existing)
			removeCount--
			if removeCount <= 0 {
				break
			}
		}
	}
	state := &vesselState{}
	d.vessels[mmsi] = state
	return state
}

func (d *Domain) normalizePosition(record plugins.RawRecord, raw rawPosition) []events.Envelope {
	if mmsiCategory(raw.MMSI) != "vessel" {
		// Base stations, aids to navigation, SAR aircraft and other non-ship
		// AIS participants never become vessel entities, however they got
		// here — normally the source's FilterMessageTypes already keeps
		// their own message types out, but a position-shaped message with a
		// non-vessel MMSI is rejected here too rather than trusted.
		return nil
	}
	// A Longitude/Latitude outside WGS84 bounds is never a real fix. This
	// also happens to be exactly how AIS spells "position not available"
	// (Longitude 181, Latitude 91 — both just past the valid range), so one
	// bounds check does double duty instead of hard-coding those constants.
	if raw.Longitude < -180 || raw.Longitude > 180 || raw.Latitude < -90 || raw.Latitude > 90 {
		return nil
	}

	// AIS "not available" sentinels: Sog 102.3 kn, Cog 360.0°, TrueHeading
	// 511. Valid ranges top out just below each sentinel (102.2, 359.9, 359),
	// so a threshold just under the sentinel is exact, not a fuzzy epsilon.
	sogKn, sogKnown := raw.SogKn, raw.SogKn < 102.25
	cogDeg, cogKnown := raw.CogDeg, raw.CogDeg < 359.95
	headingDeg, headingKnown := raw.HeadingDeg, raw.HeadingDeg <= 359

	statusText := ""
	stationary := false
	if raw.NavStatus != nil {
		statusText = navStatusText(*raw.NavStatus)
		stationary = isStationaryStatus(*raw.NavStatus)
	} else if sogKnown && sogKn <= 0.5 {
		// Class B carries no navigational-status field at all; speed is the
		// only signal available for "this craft is not currently moving".
		stationary = true
	}

	fingerprint := positionFingerprint{
		lon: int64(raw.Longitude * 1e5), lat: int64(raw.Latitude * 1e5),
		heading: -1, navStatus: -1,
	}
	if cogKnown {
		fingerprint.cog = int64(cogDeg)
	}
	if headingKnown {
		fingerprint.heading = headingDeg
	}
	if raw.NavStatus != nil {
		fingerprint.navStatus = *raw.NavStatus
	}

	validFor := validityFor(stationary)

	d.mu.Lock()
	state := d.getOrCreateLocked(raw.MMSI)
	reaffirmDue := state.lastEmittedAt.IsZero() || record.ObservedUTC.Sub(state.lastEmittedAt) >= time.Duration(float64(validFor)*reaffirmFraction)
	if state.hasFingerprint && state.fingerprint == fingerprint && !reaffirmDue {
		state.touchedAt = record.ObservedUTC
		d.mu.Unlock()
		return nil
	}
	state.hasFingerprint = true
	state.fingerprint = fingerprint
	state.lastEmittedAt = record.ObservedUTC
	state.touchedAt = record.ObservedUTC
	name, callSign, destination := state.name, state.callSign, state.destination
	shipTypeCode, imoNumber := state.shipTypeCode, state.imoNumber
	etaMonth, etaDay, etaHour, etaMinute := state.etaMonth, state.etaDay, state.etaHour, state.etaMinute
	dimA, dimB := state.dimA, state.dimB
	draughtM := state.draughtM
	d.mu.Unlock()

	source := events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}
	event := events.NewEnvelope(record.OriginalID, "maritime", "maritime.vessel", events.MessageObservation, source, record.ObservedUTC)
	event.EntityID = fmt.Sprintf("maritime:vessel:%d", raw.MMSI)
	event.Geometry = events.Point(raw.Longitude, raw.Latitude, 0)
	validUntil := record.ObservedUTC.Add(validFor)
	event.Time.ValidUntilUTC = &validUntil

	title := strings.TrimSpace(name)
	if title == "" {
		title = strings.TrimSpace(raw.MetaShipName)
	}
	if title == "" {
		title = fmt.Sprintf("MMSI %d", raw.MMSI)
	}

	category, icon := shipTypeVocabulary(shipTypeCode)
	country := countryForMMSI(raw.MMSI)

	var primary string
	switch {
	case statusText != "" && stationary:
		primary = strings.ToUpper(statusText)
	case sogKnown && cogKnown:
		primary = fmt.Sprintf("%.1f KT  //  %03.0f°", sogKn, cogDeg)
	case sogKnown:
		primary = fmt.Sprintf("%.1f KT", sogKn)
	case statusText != "":
		primary = strings.ToUpper(statusText)
	default:
		primary = "POSITION UPDATE"
	}

	var details []string
	if category != "" {
		details = append(details, category)
	}
	if country != "" {
		details = append(details, country)
	}
	if callSign != "" {
		details = append(details, callSign)
	}
	// Avoid repeating the status word verbatim when it is already shown as
	// the primary line, and skip the least informative default status.
	// statusText is only non-empty when raw.NavStatus is non-nil (Class B
	// never sets it), so the dereference below is always safe here.
	if statusText != "" && !stationary && *raw.NavStatus != 0 {
		details = append(details, strings.ToUpper(statusText))
	}

	properties := map[string]any{
		"mmsi":               raw.MMSI,
		"classB":             raw.ClassB,
		"positionAccuracy":   raw.PositionAccuracy,
		"raim":               raw.Raim,
		"visual.icon":        icon,
		"visual.markerScale": markerScale(dimA, dimB),
		"display.title":      title,
		"display.primary":    primary,
	}
	if sogKnown {
		properties["speedKn"] = sogKn
	}
	if cogKnown {
		properties["courseDeg"] = cogDeg
	}
	if headingKnown {
		properties["headingDeg"] = headingDeg
	}
	if raw.NavStatus != nil {
		properties["navigationalStatusCode"] = *raw.NavStatus
		properties["navigationalStatus"] = statusText
	}
	if raw.RateOfTurn != nil && *raw.RateOfTurn != -128 {
		// The raw ITU-R M.1371 encoded value, not degrees/minute — turning
		// rate is non-linearly encoded and this domain does not invert it.
		// Still useful to a consumer as "is it turning, and which way" via
		// the sign, or "turning sharply" for the ±127 saturation codes.
		properties["rateOfTurnCode"] = *raw.RateOfTurn
	}
	if category != "" {
		properties["vesselType"] = category
	}
	if country != "" {
		properties["flagState"] = country
	}
	if callSign != "" {
		properties["callSign"] = callSign
	}
	if imoNumber != 0 {
		properties["imoNumber"] = imoNumber
	}
	if draughtM != 0 {
		properties["draughtM"] = draughtM
	}
	// Generic kinematics for dead reckoning between updates, same convention
	// the aviation domain uses: only published while genuinely moving, so a
	// moored ship's marker does not slowly drift on a stale heading.
	if !stationary && headingKnown && sogKnown && sogKn > 0.5 {
		properties["visual.headingDeg"] = float64(headingDeg)
		properties["visual.speedMps"] = sogKn * 0.514444
	}
	if len(details) > 0 {
		properties["display.secondary"] = strings.Join(details, "  //  ")
	}
	if tertiary := destinationLine(destination, etaMonth, etaDay, etaHour, etaMinute); tertiary != "" {
		properties["display.tertiary"] = tertiary
	}
	event.Properties = properties
	measured := true
	event.Quality.Measured = &measured
	return []events.Envelope{event}
}

// markerScale gives a modest, purely cosmetic bump to vessels whose static
// data reports a substantial length overall (bow + stern offsets, dimension
// A + B), so a container ship does not render at the same size as a skiff.
// Unknown dimensions (the common case for Class B, or a vessel whose static
// data has not joined yet) fall back to the same scale aviation uses for an
// unclassified aircraft.
func markerScale(dimA, dimB int) float64 {
	length := dimA + dimB
	if length >= 200 {
		return 1.2
	}
	return 0.9
}

func destinationLine(destination string, month, day, hour, minute int) string {
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return ""
	}
	eta := etaText(month, day, hour, minute)
	if eta == "" {
		return destination
	}
	return destination + "  //  ETA " + eta
}

// etaText renders the AIS voyage ETA, honouring the ITU-R M.1371 per-field
// sentinels: Month 0 / Day 0 mean no date was set, Hour 24 / Minute 60 mean
// no time-of-day was set (0 is a legitimate midnight for either, so neither
// can use a zero-value check the way Month/Day safely do).
func etaText(month, day, hour, minute int) string {
	if month == 0 || day == 0 || month > 12 || day > 31 {
		return ""
	}
	date := fmt.Sprintf("%02d-%02d", month, day)
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return date
	}
	return fmt.Sprintf("%s %02d:%02dZ", date, hour, minute)
}
