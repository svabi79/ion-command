package aviation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func normalize(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := New().Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID:   "adsb",
		SourceInstanceID: "test",
		OriginalID:       "adsb-test-1",
		Domain:           "aviation",
		ObservedUTC:      time.Now().UTC(),
		Payload:          encoded,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected one envelope, got %d", len(messages))
	}
	return messages[0].Properties
}

func TestRouteBecomesTertiaryLine(t *testing.T) {
	properties := normalize(t, map[string]any{
		"hex": "4b1613", "callsign": "SWR23K", "lat": 47.3, "lon": 8.5,
		"altFt": 35000.0, "gsKt": 450.0,
		"routeOriginCode": "CDG", "routeOriginCity": "Paris",
		"routeDestCode": "TUN", "routeDestCity": "Tunis",
	})
	if properties["display.tertiary"] != "CDG Paris  >  TUN Tunis" {
		t.Fatalf("unexpected route line %q", properties["display.tertiary"])
	}
}

func TestRouteLineNeedsBothEndpoints(t *testing.T) {
	properties := normalize(t, map[string]any{
		"hex": "4b1613", "callsign": "SWR23K", "lat": 47.3, "lon": 8.5,
		"routeOriginCode": "CDG", "routeOriginCity": "Paris",
	})
	if _, present := properties["display.tertiary"]; present {
		t.Fatal("half a route must not render")
	}
}

func TestRouteLineSurvivesMissingCities(t *testing.T) {
	properties := normalize(t, map[string]any{
		"hex": "4b1613", "callsign": "SWR23K", "lat": 47.3, "lon": 8.5,
		"routeOriginCode": "LSZH", "routeDestCode": "EGLL",
	})
	if properties["display.tertiary"] != "LSZH  >  EGLL" {
		t.Fatalf("unexpected route line %q", properties["display.tertiary"])
	}
}
