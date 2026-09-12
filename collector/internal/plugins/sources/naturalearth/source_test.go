package naturalearth

import (
	"log/slog"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestBundledRegions(t *testing.T) {
	source, err := New(config.Source{ID: "ne", Type: "geography.naturalearth"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	records, err := source.sample()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 40 {
		t.Fatalf("expected a useful region set, got %d", len(records))
	}
	if records[0].Domain != "geography" {
		t.Fatalf("domain %s", records[0].Domain)
	}
}
