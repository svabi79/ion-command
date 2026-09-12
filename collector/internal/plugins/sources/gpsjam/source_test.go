package gpsjam

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestParseHexesKeepsMediumAndHigh(t *testing.T) {
	csv := []byte("hex,count_good_aircraft,count_bad_aircraft\n" +
		"84754e7ffffffff,20,8\n" +
		"84754e7ffffffff,50,0\n" +
		"84754e7ffffffff,1,0\n")
	// first: 8/28 = 28% high; second 0% low; third below the aircraft floor
	out := parseHexes(csv)
	if len(out) != 1 {
		t.Fatalf("got %d %#v", len(out), out)
	}
	if out[0]["level"] != "high" {
		t.Fatalf("%v", out[0])
	}
}

func TestLatestDate(t *testing.T) {
	date, err := latestDate([]byte("2026-09-10\n2026-09-11\n"))
	if err != nil || date != "2026-09-11" {
		t.Fatalf("%q %v", date, err)
	}
}

func TestSample(t *testing.T) {
	source, err := New(config.Source{ID: "g", Type: "aviation.gpsjam"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(_ context.Context, rawURL string) ([]byte, error) {
		if strings.Contains(rawURL, "manifest") {
			return []byte("2026-09-11\n"), nil
		}
		return []byte("hex,count_good_aircraft,count_bad_aircraft\n84754e7ffffffff,10,4\n"), nil
	}
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 1 {
		t.Fatalf("%v %d", err, len(records))
	}
}
