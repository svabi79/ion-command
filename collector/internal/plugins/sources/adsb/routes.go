package adsb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Route lookup: adsbdb.com resolves a callsign to its filed origin and
// destination airport. Lookups run on a single background worker, globally
// spaced like the position queries, and every answer - including "unknown" -
// is cached so a callsign is asked about at most once per TTL. The position
// pipeline never waits: a route appears on the next poll after its lookup
// completed.
const (
	routeBase        = "https://api.adsbdb.com/v0/callsign/"
	routeSpacing     = 2 * time.Second
	routeTTL         = 6 * time.Hour
	routeNegativeTTL = time.Hour
	routeQueueCap    = 512
	routeCacheCap    = 4096
	routeBackoff     = 5 * time.Minute
)

// routeGate spaces lookups across every source instance, mirroring the
// requestGate pattern used for the position queries.
var routeGate = struct {
	sync.Mutex
	last time.Time
}{}

func waitForRouteSlot(ctx context.Context) error {
	for {
		routeGate.Lock()
		wait := routeSpacing - time.Since(routeGate.last)
		if wait <= 0 {
			routeGate.last = time.Now()
			routeGate.Unlock()
			return nil
		}
		routeGate.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

type routeInfo struct {
	originCode string
	originCity string
	destCode   string
	destCity   string
	ok         bool
	fetchedAt  time.Time
}

type routeResolver struct {
	mu      sync.Mutex
	cache   map[string]routeInfo
	pending map[string]struct{}
	queue   chan string
	client  *http.Client
	logger  *slog.Logger
	// fetch is swappable for tests; returns body and HTTP status.
	fetch func(ctx context.Context, callsign string) ([]byte, int, error)
}

func newRouteResolver(logger *slog.Logger) *routeResolver {
	resolver := &routeResolver{
		cache:   make(map[string]routeInfo),
		pending: make(map[string]struct{}),
		queue:   make(chan string, routeQueueCap),
		client:  &http.Client{Timeout: 30 * time.Second},
		logger:  logger,
	}
	resolver.fetch = resolver.httpFetch
	return resolver
}

// routeFor returns the cached route for a callsign and queues a lookup when
// none is cached yet (or the entry expired). Never blocks.
func (r *routeResolver) routeFor(callsign string) (routeInfo, bool) {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	info, cached := r.cache[callsign]
	if cached {
		ttl := routeTTL
		if !info.ok {
			ttl = routeNegativeTTL
		}
		if now.Sub(info.fetchedAt) < ttl {
			return info, info.ok
		}
	}
	if _, queued := r.pending[callsign]; !queued {
		select {
		case r.queue <- callsign:
			r.pending[callsign] = struct{}{}
		default:
			// Queue full: the callsign is retried on a later poll.
		}
	}
	// Serve a stale answer while the refresh is in flight.
	return info, cached && info.ok
}

func (r *routeResolver) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case callsign := <-r.queue:
			if err := waitForRouteSlot(ctx); err != nil {
				return
			}
			body, status, err := r.fetch(ctx, callsign)
			if ctx.Err() != nil {
				return
			}
			if err != nil || status == http.StatusTooManyRequests || status == 420 {
				// Transient: leave the callsign uncached so a later poll
				// re-queues it, and give the service room to breathe.
				if err != nil {
					r.logger.Warn("route lookup failed", "callsign", callsign, "error", err)
				} else {
					r.logger.Warn("route lookup rate limited; pausing", "status", status)
				}
				r.mu.Lock()
				delete(r.pending, callsign)
				r.mu.Unlock()
				select {
				case <-ctx.Done():
					return
				case <-time.After(routeBackoff):
				}
				continue
			}
			info := routeInfo{fetchedAt: time.Now()}
			if status == http.StatusOK {
				if parsed, parseErr := parseRoute(body); parseErr == nil {
					info = parsed
					info.fetchedAt = time.Now()
				}
			}
			// Anything else (404 unknown callsign, malformed body) is a
			// negative entry: valid answer, no route known.
			r.mu.Lock()
			if len(r.cache) >= routeCacheCap {
				r.evictOldestLocked()
			}
			r.cache[callsign] = info
			delete(r.pending, callsign)
			r.mu.Unlock()
		}
	}
}

// evictOldestLocked keeps the cache bounded; called with the mutex held.
func (r *routeResolver) evictOldestLocked() {
	oldestKey := ""
	var oldestAt time.Time
	for key, entry := range r.cache {
		if oldestKey == "" || entry.fetchedAt.Before(oldestAt) {
			oldestKey, oldestAt = key, entry.fetchedAt
		}
	}
	if oldestKey != "" {
		delete(r.cache, oldestKey)
	}
}

func (r *routeResolver) httpFetch(ctx context.Context, callsign string) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, routeBase+url.PathEscape(callsign), nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	response, err := r.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return body, response.StatusCode, err
}

// parseRoute extracts the two route endpoints from an adsbdb flightroute
// response. IATA codes are preferred (what airport signage shows), the city
// over the formal airport name.
func parseRoute(body []byte) (routeInfo, error) {
	var response struct {
		Response struct {
			Flightroute struct {
				Origin      routeAirport `json:"origin"`
				Destination routeAirport `json:"destination"`
			} `json:"flightroute"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return routeInfo{}, err
	}
	origin := response.Response.Flightroute.Origin
	destination := response.Response.Flightroute.Destination
	info := routeInfo{
		originCode: origin.code(),
		originCity: origin.city(),
		destCode:   destination.code(),
		destCity:   destination.city(),
	}
	if info.originCode == "" || info.destCode == "" {
		return routeInfo{}, fmt.Errorf("route response without airport codes")
	}
	info.ok = true
	return info, nil
}

type routeAirport struct {
	Iata         string `json:"iata_code"`
	Icao         string `json:"icao_code"`
	Municipality string `json:"municipality"`
	Name         string `json:"name"`
}

func (a routeAirport) code() string {
	if a.Iata != "" {
		return a.Iata
	}
	return a.Icao
}

func (a routeAirport) city() string {
	if a.Municipality != "" {
		return a.Municipality
	}
	return a.Name
}
