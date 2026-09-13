package naturalearth

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestBundledCartography(t *testing.T) {
	source, err := New(config.Source{ID: "ne", Type: "geography.naturalearth"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	records, err := source.sample()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, record := range records {
		if record.Domain != "geography" {
			t.Fatalf("domain %s", record.Domain)
		}
		var kind struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(record.Payload, &kind); err != nil {
			t.Fatal(err)
		}
		counts[kind.Kind]++
	}
	if counts["region"] < 40 {
		t.Fatalf("expected a useful region set, got %d", counts["region"])
	}
	if counts["border"] < 200 {
		t.Fatalf("expected admin-0 borders, got %d", counts["border"])
	}
	if counts["city"] < 80 {
		t.Fatalf("expected populated places, got %d", counts["city"])
	}
	if counts["river"] < 8 {
		t.Fatalf("expected major rivers, got %d", counts["river"])
	}
	if counts["country"] < 80 {
		t.Fatalf("expected country labels, got %d", counts["country"])
	}
	if counts["peak"] < 8 {
		t.Fatalf("expected physical landmarks, got %d", counts["peak"])
	}
}
