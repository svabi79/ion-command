package events

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = 1

type MessageType string

const (
	MessageEntity       MessageType = "entity"
	MessageObservation  MessageType = "observation"
	MessageRelationship MessageType = "relationship"
	MessageTrack        MessageType = "track"
	MessageArea         MessageType = "area"
	MessageField        MessageType = "field"
	MessageVolume       MessageType = "volume"
	MessageAnnotation   MessageType = "annotation"
)

type SourceRef struct {
	PluginID   string `json:"pluginId"`
	InstanceID string `json:"instanceId"`
	OriginalID string `json:"originalId,omitempty"`
}

type TimeRange struct {
	ObservedUTC   time.Time  `json:"observedUtc"`
	ReceivedUTC   time.Time  `json:"receivedUtc"`
	ValidFromUTC  time.Time  `json:"validFromUtc"`
	ValidUntilUTC *time.Time `json:"validUntilUtc,omitempty"`
	ProcessingUTC time.Time  `json:"processingUtc"`
}

// Geometry coordinates follow GeoJSON order: longitude, latitude, altitude.
// JSON is retained to support future raster and volume encodings without a
// breaking envelope change.
type Geometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates,omitempty"`
	CRS         string          `json:"crs,omitempty"`
}

type Quality struct {
	Confidence     *float64 `json:"confidence,omitempty"`
	Classification string   `json:"classification,omitempty"`
	Measured       *bool    `json:"measured,omitempty"`
}

type RelationshipRef struct {
	Type     string `json:"type"`
	TargetID string `json:"targetId"`
}

type Envelope struct {
	SchemaVersion int               `json:"schemaVersion"`
	MessageID     string            `json:"messageId"`
	MessageType   MessageType       `json:"messageType"`
	Domain        string            `json:"domain"`
	SemanticType  string            `json:"semanticType"`
	EntityID      string            `json:"entityId,omitempty"`
	FromEntityID  string            `json:"fromEntityId,omitempty"`
	ToEntityID    string            `json:"toEntityId,omitempty"`
	TargetID      string            `json:"targetId,omitempty"`
	Source        SourceRef         `json:"source"`
	Time          TimeRange         `json:"time"`
	Geometry      Geometry          `json:"geometry"`
	Properties    map[string]any    `json:"properties,omitempty"`
	Quality       Quality           `json:"quality,omitempty"`
	Relationships []RelationshipRef `json:"relationships,omitempty"`
}

func NewEnvelope(messageID, domain, semanticType string, messageType MessageType, source SourceRef, observed time.Time) Envelope {
	now := time.Now().UTC()
	observed = observed.UTC()
	return Envelope{
		SchemaVersion: SchemaVersion,
		MessageID:     messageID,
		MessageType:   messageType,
		Domain:        domain,
		SemanticType:  semanticType,
		Source:        source,
		Time:          TimeRange{ObservedUTC: observed, ReceivedUTC: now, ValidFromUTC: observed, ProcessingUTC: now},
		Properties:    make(map[string]any),
		Geometry:      Geometry{Type: "None", CRS: "EPSG:4326"},
	}
}

func Point(longitude, latitude, altitude float64) Geometry {
	coordinates, _ := json.Marshal([]float64{longitude, latitude, altitude})
	return Geometry{Type: "Point", Coordinates: coordinates, CRS: "EPSG:4326"}
}

func GreatCircle(fromLongitude, fromLatitude, toLongitude, toLatitude float64) Geometry {
	coordinates, _ := json.Marshal([][]float64{{fromLongitude, fromLatitude}, {toLongitude, toLatitude}})
	return Geometry{Type: "GreatCircle", Coordinates: coordinates, CRS: "EPSG:4326"}
}

func (e Envelope) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %d", e.SchemaVersion)
	}
	if strings.TrimSpace(e.MessageID) == "" || strings.TrimSpace(string(e.MessageType)) == "" {
		return fmt.Errorf("messageId and messageType are required")
	}
	if strings.TrimSpace(e.Domain) == "" || strings.TrimSpace(e.SemanticType) == "" {
		return fmt.Errorf("domain and semanticType are required")
	}
	if e.Source.PluginID == "" || e.Source.InstanceID == "" {
		return fmt.Errorf("source pluginId and instanceId are required")
	}
	if e.Time.ObservedUTC.IsZero() || e.Time.ReceivedUTC.IsZero() {
		return fmt.Errorf("observedUtc and receivedUtc are required")
	}
	if e.Geometry.CRS != "" && e.Geometry.CRS != "EPSG:4326" {
		return fmt.Errorf("unsupported CRS %q", e.Geometry.CRS)
	}
	return validateGeometry(e.Geometry)
}

func validateGeometry(geometry Geometry) error {
	switch geometry.Type {
	case "None":
		return nil
	case "Point":
		var point []float64
		if err := json.Unmarshal(geometry.Coordinates, &point); err != nil || len(point) < 2 {
			return fmt.Errorf("Point requires at least longitude and latitude")
		}
		return validateLonLat(point[0], point[1])
	case "GreatCircle":
		var line [][]float64
		if err := json.Unmarshal(geometry.Coordinates, &line); err != nil || len(line) != 2 || len(line[0]) < 2 || len(line[1]) < 2 {
			return fmt.Errorf("GreatCircle requires exactly two positions")
		}
		if err := validateLonLat(line[0][0], line[0][1]); err != nil {
			return err
		}
		return validateLonLat(line[1][0], line[1][1])
	default:
		// Unknown geometry types remain forwardable and recordable by design.
		return nil
	}
}

func validateLonLat(longitude, latitude float64) error {
	if longitude < -180 || longitude > 180 || latitude < -90 || latitude > 90 {
		return fmt.Errorf("coordinate outside WGS84 bounds: lon=%v lat=%v", longitude, latitude)
	}
	return nil
}
