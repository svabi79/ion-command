// Package ais streams live vessel position and static/voyage data from
// aisstream.io (data courtesy of aisstream.io; a free API key is required —
// see docs/DATA-SOURCES.md for how to obtain one and what its terms expect).
//
// aisstream.io decodes the raw ITU-R M.1371 AIS bitstream on the server side
// and forwards one JSON envelope per AIS message. This package only extracts
// the message types needed to place vessels on the globe — position reports
// and static/voyage data — and passes their fields through unchanged, in the
// units the provider already uses (decimal degrees, knots, degrees). It does
// not interpret AIS "not available" sentinels, map codes to vocabulary, or
// decide which MMSIs are vessels: the source returns raw records only, and
// the maritime domain owns all of that interpretation.
//
// The struct shapes below mirror the OpenAPI-generated model definitions
// aisstream.io publishes at github.com/aisstream/ais-message-models (field
// names and required/optional-ness taken from that schema), cross-checked
// against the worked examples on https://aisstream.io/documentation. Field
// names are matched case-insensitively by encoding/json, which also covers
// the lower-cased MetaData variants ("latitude"/"longitude") that some older
// captured samples show alongside the documented capitalised ones.
package ais

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

// streamEnvelope is the top-level object aisstream.io sends for every
// WebSocket frame: {"MessageType": "...", "MetaData": {...}, "Message":
// {"<MessageType>": {...}}}. Only the message types this source subscribes
// to (see subscribeMessage in source.go) are ever expected to arrive, but
// unrecognised ones (e.g. a bare SubscriptionConfirmation) decode cleanly to
// a struct with every pointer nil and are simply skipped.
type streamEnvelope struct {
	MessageType string         `json:"MessageType"`
	MetaData    streamMetaData `json:"MetaData"`
	Message     streamMessage  `json:"Message"`
}

// streamMetaData carries a best-effort ship name alongside every message,
// including position reports that arrive long before this vessel's next
// static/voyage report. It is a convenience echo, not authoritative; the
// domain prefers a joined static-data name and falls back to this one.
type streamMetaData struct {
	MMSI     int64  `json:"MMSI"`
	ShipName string `json:"ShipName"`
}

// streamMessage is a union: exactly one field is non-nil, selected by
// MessageType, mirroring aisstream's own AisStreamMessageMessage model.
type streamMessage struct {
	PositionReport               *positionReportMessage       `json:"PositionReport,omitempty"`
	StandardClassBPositionReport *classBPositionReportMessage `json:"StandardClassBPositionReport,omitempty"`
	ShipStaticData               *shipStaticDataMessage       `json:"ShipStaticData,omitempty"`
	StaticDataReport             *staticDataReportMessage     `json:"StaticDataReport,omitempty"`
}

// positionReportMessage mirrors aisstream's PositionReport model (AIS
// message types 1/2/3, Class A). Values are already decoded to engineering
// units by the provider (decimal degrees, knots, degrees), but AIS's
// "not available" sentinels survive that conversion unchanged: Longitude
// 181 / Latitude 91, Sog 102.3, Cog 360.0, TrueHeading 511, RateOfTurn -128.
// The domain, not this decoder, is responsible for recognising them.
type positionReportMessage struct {
	UserID             int64   `json:"UserID"`
	NavigationalStatus int     `json:"NavigationalStatus"`
	RateOfTurn         int     `json:"RateOfTurn"`
	Sog                float64 `json:"Sog"`
	PositionAccuracy   bool    `json:"PositionAccuracy"`
	Longitude          float64 `json:"Longitude"`
	Latitude           float64 `json:"Latitude"`
	Cog                float64 `json:"Cog"`
	TrueHeading        int     `json:"TrueHeading"`
	Raim               bool    `json:"Raim"`
}

// classBPositionReportMessage mirrors StandardClassBPositionReport (AIS
// message type 18). Class B transponders (small craft, fishing boats,
// pleasure yachts) never report NavigationalStatus or RateOfTurn — those
// fields do not exist in this message at all, not merely as sentinels.
type classBPositionReportMessage struct {
	UserID           int64   `json:"UserID"`
	Sog              float64 `json:"Sog"`
	PositionAccuracy bool    `json:"PositionAccuracy"`
	Longitude        float64 `json:"Longitude"`
	Latitude         float64 `json:"Latitude"`
	Cog              float64 `json:"Cog"`
	TrueHeading      int     `json:"TrueHeading"`
	Raim             bool    `json:"Raim"`
}

// dimensionMessage mirrors ShipStaticDataDimension: distances in metres from
// the reported position to the bow/stern/port/starboard. All-zero means the
// transponder never had its dimensions configured.
type dimensionMessage struct {
	A int `json:"A"`
	B int `json:"B"`
	C int `json:"C"`
	D int `json:"D"`
}

// etaMessage mirrors ShipStaticDataEta. Per ITU-R M.1371 the sentinels are
// per-field and NOT uniformly zero: Month 0 and Day 0 both mean "not given",
// but Hour's sentinel is 24 and Minute's is 60 (0 is a legitimate midnight).
type etaMessage struct {
	Month  int `json:"Month"`
	Day    int `json:"Day"`
	Hour   int `json:"Hour"`
	Minute int `json:"Minute"`
}

// shipStaticDataMessage mirrors ShipStaticData (AIS message type 5, Class A
// static/voyage data). Class A vessels send this only once every few
// minutes, on a completely independent schedule from their position
// reports, which is why the domain has to join it from a cache instead of
// reading it straight off the same message as the fix.
type shipStaticDataMessage struct {
	UserID               int64            `json:"UserID"`
	ImoNumber            int              `json:"ImoNumber"`
	CallSign             string           `json:"CallSign"`
	Name                 string           `json:"Name"`
	Type                 int              `json:"Type"`
	Dimension            dimensionMessage `json:"Dimension"`
	Eta                  etaMessage       `json:"Eta"`
	MaximumStaticDraught float64          `json:"MaximumStaticDraught"`
	Destination          string           `json:"Destination"`
}

// staticDataReportMessage mirrors StaticDataReport (AIS message type 24,
// Class B static data). Class B equipment splits the same information the
// aviation lets across two independent part messages sent minutes apart:
// Part A (PartNumber == false) carries only the name, Part B carries type,
// call sign and dimensions. The domain merges both parts into one cache
// entry as they arrive, same as the Class A case.
type staticDataReportMessage struct {
	UserID     int64 `json:"UserID"`
	PartNumber bool  `json:"PartNumber"`
	ReportA    struct {
		Name string `json:"Name"`
	} `json:"ReportA"`
	ReportB struct {
		ShipType  int              `json:"ShipType"`
		CallSign  string           `json:"CallSign"`
		Dimension dimensionMessage `json:"Dimension"`
	} `json:"ReportB"`
}

// ParseFrame decodes one aisstream.io WebSocket frame into raw records. A
// frame produces zero records when it is not one of the message types this
// source cares about (subscription confirmations, or any other AIS message
// type the operator's FilterMessageTypes did not request) — that is normal
// stream traffic, not an error. A frame that fails to parse as JSON at all
// is an error; the caller counts and rate-limits logging of those.
func ParseFrame(frame []byte, sourceInstanceID string) ([]plugins.RawRecord, error) {
	var envelope streamEnvelope
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return nil, fmt.Errorf("decode ais frame: %w", err)
	}
	now := time.Now().UTC()
	switch {
	case envelope.Message.PositionReport != nil:
		message := envelope.Message.PositionReport
		return recordFor(sourceInstanceID, now, message.UserID, positionPayload(message.UserID, false, message.Longitude, message.Latitude, message.Sog, message.Cog, message.TrueHeading, message.PositionAccuracy, message.Raim, &message.NavigationalStatus, &message.RateOfTurn, envelope.MetaData.ShipName)), nil
	case envelope.Message.StandardClassBPositionReport != nil:
		message := envelope.Message.StandardClassBPositionReport
		return recordFor(sourceInstanceID, now, message.UserID, positionPayload(message.UserID, true, message.Longitude, message.Latitude, message.Sog, message.Cog, message.TrueHeading, message.PositionAccuracy, message.Raim, nil, nil, envelope.MetaData.ShipName)), nil
	case envelope.Message.ShipStaticData != nil:
		message := envelope.Message.ShipStaticData
		payload := map[string]any{
			"kind":         "static",
			"mmsi":         message.UserID,
			"name":         strings.TrimSpace(message.Name),
			"callSign":     strings.TrimSpace(message.CallSign),
			"shipTypeCode": message.Type,
			"imoNumber":    message.ImoNumber,
			"destination":  strings.TrimSpace(message.Destination),
			"etaMonth":     message.Eta.Month,
			"etaDay":       message.Eta.Day,
			"etaHour":      message.Eta.Hour,
			"etaMinute":    message.Eta.Minute,
			"dimA":         message.Dimension.A,
			"dimB":         message.Dimension.B,
			"dimC":         message.Dimension.C,
			"dimD":         message.Dimension.D,
			"draughtM":     message.MaximumStaticDraught,
		}
		return recordFor(sourceInstanceID, now, message.UserID, payload), nil
	case envelope.Message.StaticDataReport != nil:
		message := envelope.Message.StaticDataReport
		payload := map[string]any{"kind": "static", "mmsi": message.UserID}
		if !message.PartNumber {
			payload["name"] = strings.TrimSpace(message.ReportA.Name)
		} else {
			payload["callSign"] = strings.TrimSpace(message.ReportB.CallSign)
			payload["shipTypeCode"] = message.ReportB.ShipType
			payload["dimA"] = message.ReportB.Dimension.A
			payload["dimB"] = message.ReportB.Dimension.B
			payload["dimC"] = message.ReportB.Dimension.C
			payload["dimD"] = message.ReportB.Dimension.D
		}
		return recordFor(sourceInstanceID, now, message.UserID, payload), nil
	default:
		// SubscriptionConfirmation and any message type not requested via
		// FilterMessageTypes. Not an error.
		return nil, nil
	}
}

func positionPayload(mmsi int64, classB bool, lon, lat, sog, cog float64, heading int, positionAccuracy, raim bool, navStatus, rateOfTurn *int, metaShipName string) map[string]any {
	payload := map[string]any{
		"kind":             "position",
		"mmsi":             mmsi,
		"classB":           classB,
		"lon":              lon,
		"lat":              lat,
		"sogKn":            sog,
		"cogDeg":           cog,
		"headingDeg":       heading,
		"positionAccuracy": positionAccuracy,
		"raim":             raim,
		"metaShipName":     strings.TrimSpace(metaShipName),
	}
	if navStatus != nil {
		payload["navStatus"] = *navStatus
	}
	if rateOfTurn != nil {
		payload["rateOfTurn"] = *rateOfTurn
	}
	return payload
}

func recordFor(sourceInstanceID string, observed time.Time, mmsi int64, payload map[string]any) []plugins.RawRecord {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return []plugins.RawRecord{{
		SourcePluginID:   "ais",
		SourceInstanceID: sourceInstanceID,
		OriginalID:       fmt.Sprintf("ais-%s-%d-%d", sourceInstanceID, mmsi, observed.UnixNano()),
		Domain:           "maritime",
		ObservedUTC:      observed,
		Payload:          encoded,
	}}
}
