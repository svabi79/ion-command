package portwatch

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

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
		if strings.Contains(rawURL, "disruptions") {
			return []byte(`{"features":[{"attributes":{"eventid":99,"eventname":"Red Sea","eventtype":"Conflict","alertlevel":"Red","country":"Yemen","lat":14.5,"long":42.8}}]}`), nil
		}
		return []byte(`{"features":[{"attributes":{"portid":"chokepoint1","portname":"Suez Canal"},"geometry":{"x":32.3,"y":30.5}}]}`), nil
	}
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
}

func TestArcgisTimestamp(t *testing.T) {
	got := arcgisTimestamp(time.Date(2026, 8, 13, 5, 51, 0, 0, time.UTC))
	if got != "2026-08-13 05:51:00" {
		t.Fatalf("%q", got)
	}
}
