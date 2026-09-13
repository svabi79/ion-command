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

func TestEmergencySquawkIsStickyAndLongLived(t *testing.T) {
	domain := New()
	observed := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	first, err := domain.Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "opensky", SourceInstanceID: "test", OriginalID: "a",
		Domain: "aviation", ObservedUTC: observed,
		Payload: []byte(`{"hex":"abc123","callsign":"TEST1","lat":47,"lon":8,"altFt":35000,"gsKt":400,"squawk":"7700"}`),
	})
	if err != nil || len(first) != 1 {
		t.Fatalf("first: %v %#v", err, first)
	}
	if first[0].Properties["visual.emergency"] != "EMERGENCY" {
		t.Fatalf("7700 must raise EMERGENCY: %#v", first[0].Properties)
	}
	if first[0].Time.ValidUntilUTC == nil || first[0].Time.ValidUntilUTC.Sub(observed) < 2*time.Hour {
		t.Fatal("emergency validity must stay at least two hours")
	}
	later, err := domain.Normalize(context.Background(), plugins.RawRecord{
		SourcePluginID: "opensky", SourceInstanceID: "test", OriginalID: "b",
		Domain: "aviation", ObservedUTC: observed.Add(20 * time.Minute),
		Payload: []byte(`{"hex":"ABC123","callsign":"TEST1","lat":47.1,"lon":8.1,"altFt":34000,"gsKt":390,"onGround":true}`),
	})
	if err != nil || len(later) != 1 {
		t.Fatalf("later: %v %#v", err, later)
	}
	if later[0].Properties["visual.emergency"] != "EMERGENCY" {
		t.Fatalf("null squawk must not clear a remembered 7700: %#v", later[0].Properties)
	}
}
