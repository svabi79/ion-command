package portwatch

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestSampleJoinsDailyCounts(t *testing.T) {
	source, err := New(config.Source{ID: "p", Type: "maritime.portwatch"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context, rawURL string) ([]byte, error) {
		if strings.Contains(rawURL, "Daily") {
			return []byte(`{"features":[{"attributes":{"portid":"chokepoint1","n_total":42}}]}`), nil
		}
		return []byte(`{"features":[{"attributes":{"portid":"chokepoint1","portname":"Suez Canal"},"geometry":{"x":32.3,"y":30.5}}]}`), nil
	}
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
}
