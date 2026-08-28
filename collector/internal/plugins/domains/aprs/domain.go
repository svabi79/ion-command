// Package aprs normalizes raw APRS-IS packet lines into canonical
// per-station/object Point observations. It owns all APRS vocabulary -
// callsigns, symbols, packet types - so that nothing outside this package
// needs to know what any of that means; the aprsis source only hands it
// text lines.
//
// Supported packet types: uncompressed and compressed position reports
// (data type indicators "!", "=", "/", "@"), Object reports (";"), Item
// reports (")"), and Mic-E position/symbol/status reports ("`", "'" - see
// mice.go for why Mic-E course/speed is not decoded). Everything else
// (status ">", messages ":", telemetry "T#", positionless weather "_",
// third-party "}", queries, raw NMEA "$", user-defined data, and any line
// that fails to parse) is skipped cleanly: Normalize returns zero envelopes
// and no error, since APRS-IS carrying packet types this domain does not
// plot is expected, ordinary traffic, not a fault.
package aprs

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

// positionValidity is how long a station/object stays on the globe after
// its last reported position: long enough that a fixed station beaconing
// every 10-30 minutes does not flicker, short enough that a feed outage is
// visible within the hour rather than leaving stale markers forever.
const positionValidity = 60 * time.Minute

// maxTrackedEntities bounds the last-position dedup cache; APRS-IS is a
// firehose and the same packet is routinely delivered more than once via
// different digipeater/igate paths.
const maxTrackedEntities = 200000

type Domain struct {
	mu       sync.Mutex
	lastFix  map[string]lastPosition
	capacity int
}

type lastPosition struct {
	lon, lat float64
}

func New() *Domain {
	return &Domain{lastFix: make(map[string]lastPosition), capacity: maxTrackedEntities}
}

func (d *Domain) ID() string     { return "domain.aprs" }
func (d *Domain) Domain() string { return "aprs" }

type rawLine struct {
	Raw string `json:"raw"`
}

func (d *Domain) Normalize(_ context.Context, record plugins.RawRecord) ([]events.Envelope, error) {
	var raw rawLine
	if err := json.Unmarshal(record.Payload, &raw); err != nil {
		return nil, fmt.Errorf("decode aprs.is record: %w", err)
	}
	packet, ok := parseTNC2(raw.Raw)
	if !ok {
		return nil, nil
	}
	f, ok := decode(packet)
	if !ok {
		return nil, nil
	}

	entityID, title := entityIdentity(packet, f)
	if f.live {
		// Suppress the extremely common case of the identical position
		// being redelivered via a different digipeater/igate path; a real
		// state change (including a kill report) always gets through.
		if !d.positionChanged(entityID, f.lon, f.lat) {
			return nil, nil
		}
	} else {
		d.forgetPosition(entityID)
	}

	return []events.Envelope{d.buildEnvelope(record, packet, f, entityID, title)}, nil
}

// decode dispatches on the APRS data type indicator (the info field's first
// byte). See the package comment for the full list of what is and is not
// supported.
func decode(packet tnc2Packet) (fix, bool) {
	info := packet.info
	if info == "" {
		return fix{}, false
	}
	switch info[0] {
	case '!', '=':
		f, ok := decodePositionBody(info[1:])
		if !ok {
			return fix{}, false
		}
		return asStation(f), true
	case '/', '@':
		if len(info) < 8 {
			return fix{}, false
		}
		// Bytes 1-7 are a DHM/HMS timestamp. Its value is not used (see
		// the ObservedUTC comment in buildEnvelope below); it is only
		// skipped over to find where the position body starts.
		f, ok := decodePositionBody(info[8:])
		if !ok {
			return fix{}, false
		}
		return asStation(f), true
	case ';':
		return decodeObject(info)
	case ')':
		return decodeItem(info)
	case '`', '\'':
		f, ok := decodeMicE(packet.dest, info[1:])
		if !ok {
			return fix{}, false
		}
		return asStation(f), true
	default:
		// Status ">", messages ":", telemetry "T", positionless/raw
		// weather "_"/"#"/"$"/"*", third-party "}", queries "?" and
		// anything else this domain does not plot.
		return fix{}, false
	}
}

func asStation(f fix) fix {
	f.kind, f.live = "station", true
	return f
}

// entityIdentity derives the stable per-entity ID and display title. A
// plain position report (or Mic-E, which carries the real callsign in the
// AX.25 source field even though its destination field is repurposed for
// position data) is identified by its transmitting callsign; an Object or
// Item is identified by its own name, independent of whichever station is
// currently relaying it - replacing or killing an Object is a normal part
// of the protocol (APRS101 chapter 11).
func entityIdentity(packet tnc2Packet, f fix) (entityID, title string) {
	switch f.kind {
	case "object":
		return "aprs:object:" + f.name, f.name
	case "item":
		return "aprs:item:" + f.name, f.name
	default:
		call := strings.ToUpper(strings.TrimSpace(packet.source))
		return "aprs:station:" + call, call
	}
}

func (d *Domain) positionChanged(entityID string, lon, lat float64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	prev, seen := d.lastFix[entityID]
	if seen && prev.lon == lon && prev.lat == lat {
		return false
	}
	if !seen && len(d.lastFix) >= d.capacity {
		evict := d.capacity / 10
		for existing := range d.lastFix {
			delete(d.lastFix, existing)
			evict--
			if evict <= 0 {
				break
			}
		}
	}
	d.lastFix[entityID] = lastPosition{lon: lon, lat: lat}
	return true
}

func (d *Domain) forgetPosition(entityID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.lastFix, entityID)
}

func (d *Domain) buildEnvelope(record plugins.RawRecord, packet tnc2Packet, f fix, entityID, title string) events.Envelope {
	source := events.SourceRef{PluginID: record.SourcePluginID, InstanceID: record.SourceInstanceID, OriginalID: record.OriginalID}
	semanticType := "aprs.station"
	if f.kind != "station" {
		semanticType = "aprs.object"
	}
	// ObservedUTC is the source's own packet-receipt time, not any
	// timestamp embedded in the APRS payload: embedded timestamps are
	// day/hour/minute or hour/minute/second only (no date), and may be
	// zulu or the sending station's unstated local time, so treating the
	// receive time as "observed" is both simpler and more trustworthy for
	// what is, in practice, a near-real-time feed.
	event := events.NewEnvelope(record.OriginalID, "aprs", semanticType, events.MessageObservation, source, record.ObservedUTC)
	event.EntityID = entityID
	altitude := 0.0
	if f.hasAlt {
		altitude = f.altMeters
	}
	event.Geometry = events.Point(f.lon, f.lat, altitude)

	validFor := positionValidity
	if f.kind != "station" && !f.live {
		validFor = 0 // a kill report should leave the globe immediately
	}
	validUntil := record.ObservedUTC.Add(validFor)
	event.Time.ValidUntilUTC = &validUntil

	symbol := lookupSymbol(f.symTable, f.symCode)
	primary := symbol.label
	if courseSpeed := courseSpeedText(f); courseSpeed != "" {
		primary += "  //  " + courseSpeed
	}

	var secondary []string
	if f.hasAlt {
		secondary = append(secondary, fmt.Sprintf("Alt %.0f ft", f.altMeters/feetToMeters))
	}
	if f.micE != nil && f.micE.label != "" {
		secondary = append(secondary, f.micE.label)
	}
	if f.comment != "" {
		secondary = append(secondary, f.comment)
	}

	props := map[string]any{
		"display.title":   title,
		"display.primary": primary,
		"symbolTable":     string(f.symTable),
		"symbolCode":      string(f.symCode),
		"visual.icon":     symbol.icon,
	}
	if len(secondary) > 0 {
		props["display.secondary"] = strings.Join(secondary, "  //  ")
	}
	if hop := packet.lastPathHop(); hop != "" {
		props["display.tertiary"] = "via " + hop
	}
	if f.hasCourse {
		props["visual.headingDeg"] = f.courseDeg
	}
	if f.hasSpeed {
		props["visual.speedMps"] = f.speedKts * knotsToMps
	}
	if f.hasAlt {
		// True altitude is imperceptible at globe scale, same reasoning as
		// the aviation domain; reuse its exact exaggeration factor so the
		// renderer's existing handling needs no APRS-specific branch.
		props["visual.altitudeScale"] = 12
	}
	if f.kind != "station" {
		props["objectClass"] = f.kind
		props["live"] = f.live
	}
	if f.micE != nil {
		props["micEMessageType"] = f.micE.kind
		if f.micE.kind == "emergency" {
			props["display.title"] = title + "  //  EMERGENCY"
			props["visual.tint"] = "1.0,0.15,0.1"
			props["visual.markerScale"] = 2.0
			props["visual.emergency"] = "EMERGENCY"
		}
	}
	event.Properties = props
	measured := true
	event.Quality.Measured = &measured
	return event
}

func courseSpeedText(f fix) string {
	var parts []string
	if f.hasCourse {
		parts = append(parts, fmt.Sprintf("%03.0f%s", f.courseDeg, degreeSign))
	}
	if f.hasSpeed {
		parts = append(parts, fmt.Sprintf("%.0f kt", f.speedKts))
	}
	return strings.Join(parts, " ")
}

const degreeSign = "°"
