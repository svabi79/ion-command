package adsb

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// Captured from api.adsbdb.com/v0/callsign/AFR1084 on 2026-07-23.
const routeResponseLive = `{"response":{"flightroute":{"callsign":"AFR1084","callsign_icao":"AFR1084","callsign_iata":"AF1084","airline":{"name":"Air France","icao":"AFR","iata":"AF","country":"France","country_iso":"FR","callsign":"AIRFRANS"},"origin":{"country_iso_name":"FR","country_name":"France","elevation":392,"iata_code":"CDG","icao_code":"LFPG","latitude":49.012798,"longitude":2.55,"municipality":"Paris","name":"Charles de Gaulle International Airport"},"destination":{"country_iso_name":"TN","country_name":"Tunisia","elevation":22,"iata_code":"TUN","icao_code":"DTTA","latitude":36.851,"longitude":10.227,"municipality":"Tunis","name":"Tunis Carthage International Airport"}}}}`

func TestParseRouteLiveCapture(t *testing.T) {
	info, err := parseRoute([]byte(routeResponseLive))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ok {
		t.Fatal("expected a usable route")
	}
	if info.originCode != "CDG" || info.originCity != "Paris" {
		t.Fatalf("unexpected origin %q %q", info.originCode, info.originCity)
	}
	if info.destCode != "TUN" || info.destCity != "Tunis" {
		t.Fatalf("unexpected destination %q %q", info.destCode, info.destCity)
	}
}

func TestParseRouteRejectsUnknownCallsignBody(t *testing.T) {
	if _, err := parseRoute([]byte(`{"response":"unknown callsign"}`)); err == nil {
		t.Fatal("string response must not produce a route")
	}
}

func TestResolverCachesPositiveAndNegativeAnswers(t *testing.T) {
	resolver := newRouteResolver(slog.Default())
	var fetches atomic.Int32
	resolver.fetch = func(_ context.Context, callsign string) (routeFetchResult, error) {
		fetches.Add(1)
		if callsign == "AFR1084" {
			return routeFetchResult{body: []byte(routeResponseLive), status: http.StatusOK}, nil
		}
		return routeFetchResult{body: []byte(`{"response":"unknown callsign"}`), status: http.StatusNotFound}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go resolver.run(ctx)

	// First ask: nothing cached, lookup queued.
	if _, known := resolver.routeFor("AFR1084"); known {
		t.Fatal("route must not be known before the lookup ran")
	}
	waitUntil(t, func() bool { _, known := resolver.routeFor("AFR1084"); return known })
	route, _ := resolver.routeFor("AFR1084")
	if route.originCode != "CDG" || route.destCode != "TUN" {
		t.Fatalf("unexpected cached route %+v", route)
	}

	// Unknown callsigns are negative-cached: asked once, then silent.
	resolver.routeFor("XYZ999")
	waitUntil(t, func() bool {
		resolver.mu.Lock()
		defer resolver.mu.Unlock()
		_, cached := resolver.cache["XYZ999"]
		return cached
	})
	if _, known := resolver.routeFor("XYZ999"); known {
		t.Fatal("unknown callsign must not report a route")
	}
	before := fetches.Load()
	resolver.routeFor("XYZ999")
	time.Sleep(50 * time.Millisecond)
	if after := fetches.Load(); after != before {
		t.Fatalf("negative cache must suppress repeat lookups (%d -> %d)", before, after)
	}
}

func TestResolverQueueFullDropsInsteadOfBlocking(t *testing.T) {
	resolver := newRouteResolver(slog.Default())
	// No worker running: fill the queue completely...
	for i := 0; i < routeQueueCap; i++ {
		resolver.routeFor(fmt.Sprintf("FULL%04d", i))
	}
	// ...and the next ask must return immediately, unqueued and unknown.
	done := make(chan struct{})
	go func() {
		resolver.routeFor("OVERFLOW1")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("routeFor blocked on a full queue")
	}
	resolver.mu.Lock()
	_, queued := resolver.pending["OVERFLOW1"]
	pendingCount := len(resolver.pending)
	resolver.mu.Unlock()
	if queued {
		t.Fatal("overflow callsign must not be marked pending")
	}
	if pendingCount != routeQueueCap {
		t.Fatalf("pending must match the queue bound, got %d", pendingCount)
	}
}

func TestResolverHonorsRetryAfterThenRecovers(t *testing.T) {
	resolver := newRouteResolver(slog.Default())
	resolver.rateLimitPause = 10 * time.Second // must NOT be used: Retry-After wins
	var fetches atomic.Int32
	resolver.fetch = func(_ context.Context, _ string) (routeFetchResult, error) {
		if fetches.Add(1) == 1 {
			return routeFetchResult{status: http.StatusTooManyRequests, retryAfter: 50 * time.Millisecond}, nil
		}
		return routeFetchResult{body: []byte(routeResponseLive), status: http.StatusOK}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go resolver.run(ctx)

	// Keep asking: the first answer is a 429, the retry after the server's
	// 50 ms pause succeeds. With the 10 s fallback pause this would time out.
	waitUntil(t, func() bool { _, known := resolver.routeFor("AFR1084"); return known })
	if fetches.Load() < 2 {
		t.Fatalf("expected a retry after the rate limit, got %d fetches", fetches.Load())
	}
}

func TestResolverTransportErrorRetriesQuickly(t *testing.T) {
	resolver := newRouteResolver(slog.Default())
	resolver.errorPause = 20 * time.Millisecond
	resolver.rateLimitPause = time.Hour // a transport error must not use this
	var fetches atomic.Int32
	resolver.fetch = func(_ context.Context, _ string) (routeFetchResult, error) {
		if fetches.Add(1) == 1 {
			return routeFetchResult{}, fmt.Errorf("dial tcp: no such host")
		}
		return routeFetchResult{body: []byte(routeResponseLive), status: http.StatusOK}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go resolver.run(ctx)

	waitUntil(t, func() bool { _, known := resolver.routeFor("AFR1084"); return known })
	if fetches.Load() < 2 {
		t.Fatalf("expected a quick retry after the transport error, got %d fetches", fetches.Load())
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}
