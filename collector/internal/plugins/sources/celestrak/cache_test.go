package celestrak

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

// Runs the test inside a scratch directory so the cache it writes cannot
// touch the operator's real one.
func inScratchDir(t *testing.T) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
}

func quietSource(t *testing.T, id string) *Source {
	t.Helper()
	source, err := New(config.Source{ID: id, Type: "orbital.celestrak", Enabled: true},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestTLECacheRoundTripAndExpiry(t *testing.T) {
	inScratchDir(t)
	body, err := os.ReadFile(mustFixturePath(t))
	if err != nil {
		t.Fatal(err)
	}
	source := quietSource(t, "cache-test")

	if _, _, ok := source.loadTLECache(); ok {
		t.Fatal("loaded a cache that was never written")
	}
	source.storeTLECache(body)
	cached, age, ok := source.loadTLECache()
	if !ok {
		t.Fatal("cache did not come back after being written")
	}
	if len(ParseTLESets(cached)) == 0 {
		t.Fatal("cached bytes do not parse as TLEs")
	}
	if age > time.Minute {
		t.Fatalf("a cache just written should be fresh, age was %s", age)
	}

	// Stale elements are worse than none: SGP4 drifts, and a fortnight-old
	// set would predict passes that are minutes and kilometres out.
	source.now = func() time.Time { return time.Now().Add(tleCacheMaxAge + time.Hour) }
	if _, _, ok := source.loadTLECache(); ok {
		t.Fatal("a cache older than the limit was accepted")
	}
}

// The point of the cache: when the service is unreachable the source must
// still come up with satellites, rather than sitting empty until the network
// returns.
func TestStartFallsBackToCacheWhenFetchFails(t *testing.T) {
	inScratchDir(t)
	body, err := os.ReadFile(mustFixturePath(t))
	if err != nil {
		t.Fatal(err)
	}
	// Seed the cache the way a previous successful run would have.
	quietSource(t, "fallback-test").storeTLECache(body)

	source := quietSource(t, "fallback-test")
	source.fetch = func(context.Context) ([]byte, error) {
		return nil, errors.New("dial tcp: connection timed out")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output := make(chan plugins.RawRecord, 64)
	done := make(chan error, 1)
	go func() { done <- source.Start(ctx, output) }()

	select {
	case record := <-output:
		if record.Domain != "orbital" {
			t.Fatalf("unexpected record domain %q", record.Domain)
		}
	case <-ctx.Done():
		t.Fatal("no records emitted: the source stayed empty although a usable cache existed")
	}
	cancel()
	<-done
}

// Resolved at package load, before any test changes directory, so the
// fixture is still findable from a scratch working directory.
var fixtureAbsolutePath = func() string {
	absolute, err := filepath.Abs(filepath.Join("testdata", "amateur_live.tle"))
	if err != nil {
		return filepath.Join("testdata", "amateur_live.tle")
	}
	return absolute
}()

func mustFixturePath(t *testing.T) string {
	t.Helper()
	return fixtureAbsolutePath
}
