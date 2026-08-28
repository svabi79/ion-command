package ais

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func validConfig() config.Source {
	return config.Source{
		ID: "test", Type: "ais.aisstream", ApiKey: "test-key",
		BoundingBoxes: []config.BoundingBox{{MinLatitude: 25.0, MaxLatitude: 26.0, MinLongitude: -81.0, MaxLongitude: -80.0}},
	}
}

func TestNewRequiresApiKey(t *testing.T) {
	cfg := validConfig()
	cfg.ApiKey = ""
	if _, err := New(cfg, slog.Default()); err == nil {
		t.Fatal("missing apiKey must be rejected")
	}
}

func TestNewRequiresBoundingBox(t *testing.T) {
	cfg := validConfig()
	cfg.BoundingBoxes = nil
	if _, err := New(cfg, slog.Default()); err == nil {
		t.Fatal("missing boundingBoxes must be rejected")
	}
}

func TestNewRejectsInvertedBoundingBox(t *testing.T) {
	cfg := validConfig()
	cfg.BoundingBoxes = []config.BoundingBox{{MinLatitude: 26.0, MaxLatitude: 25.0, MinLongitude: -81.0, MaxLongitude: -80.0}}
	if _, err := New(cfg, slog.Default()); err == nil {
		t.Fatal("min >= max latitude must be rejected")
	}
}

func TestNewRejectsOutOfBoundsBoundingBox(t *testing.T) {
	cfg := validConfig()
	cfg.BoundingBoxes = []config.BoundingBox{{MinLatitude: -95.0, MaxLatitude: 25.0, MinLongitude: -81.0, MaxLongitude: -80.0}}
	if _, err := New(cfg, slog.Default()); err == nil {
		t.Fatal("out-of-range latitude must be rejected")
	}
}

func TestNewAcceptsBrokerOverride(t *testing.T) {
	cfg := validConfig()
	cfg.Broker = "wss://example.test/stream"
	source, err := New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if source.endpoint != "wss://example.test/stream" {
		t.Fatalf("broker override not applied: %s", source.endpoint)
	}
}

func TestNewDefaultsToPublicEndpoint(t *testing.T) {
	source, err := New(validConfig(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if source.endpoint != defaultEndpoint {
		t.Fatalf("unexpected default endpoint: %s", source.endpoint)
	}
	if source.Type() != "ais.aisstream" || source.ID() != "test" {
		t.Fatalf("unexpected identity: type=%s id=%s", source.Type(), source.ID())
	}
}

// The subscription message is the one contract aisstream.io actually
// enforces at connect time (required APIKey/BoundingBoxes, a 3-second
// deadline to send it); this locks its shape to the documented format.
func TestSubscribeMessageShape(t *testing.T) {
	source, err := New(validConfig(), slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := source.subscribeMessage()
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		APIKey             string         `json:"APIKey"`
		BoundingBoxes      [][][2]float64 `json:"BoundingBoxes"`
		FilterMessageTypes []string       `json:"FilterMessageTypes"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.APIKey != "test-key" {
		t.Fatalf("unexpected api key: %s", decoded.APIKey)
	}
	if len(decoded.BoundingBoxes) != 1 {
		t.Fatalf("expected one bounding box, got %d", len(decoded.BoundingBoxes))
	}
	box := decoded.BoundingBoxes[0]
	if box[0] != [2]float64{25.0, -81.0} || box[1] != [2]float64{26.0, -80.0} {
		t.Fatalf("unexpected bounding box corners: %v", box)
	}
	wantTypes := map[string]bool{"PositionReport": false, "StandardClassBPositionReport": false, "ShipStaticData": false, "StaticDataReport": false}
	if len(decoded.FilterMessageTypes) != len(wantTypes) {
		t.Fatalf("unexpected message type filter: %v", decoded.FilterMessageTypes)
	}
	for _, name := range decoded.FilterMessageTypes {
		if _, ok := wantTypes[name]; !ok {
			t.Fatalf("unexpected message type in filter: %s", name)
		}
		wantTypes[name] = true
	}
	for name, seen := range wantTypes {
		if !seen {
			t.Fatalf("expected message type %s to be subscribed", name)
		}
	}
	// Base stations, aids to navigation and SAR aircraft must never be
	// subscribed to — their message types must not appear in the filter.
	for _, forbidden := range []string{"BaseStationReport", "AidsToNavigationReport", "StandardSearchAndRescueAircraftReport"} {
		if _, present := wantTypes[forbidden]; present {
			t.Fatalf("non-vessel message type must never be requested: %s", forbidden)
		}
	}
}
