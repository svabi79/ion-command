package opensky

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func fixtureStates(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/states_live.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return body
}

func newOAuthSource(t *testing.T, apiBase, tokenURL string) *Source {
	t.Helper()
	source, err := New(config.Source{
		ID:           "test",
		Type:         "aviation.opensky",
		Enabled:      true,
		ClientID:     "cid",
		ClientSecret: "csecret",
		PollSeconds:  600,
		Broker:       apiBase,
	}, testLogger())
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	source.tokenURL = tokenURL
	source.now = func() time.Time { return time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC) }
	return source
}

func TestOAuthTokenFetchAndCache(t *testing.T) {
	var tokenCalls atomic.Int32
	var sawGrant atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("grant_type") == "client_credentials" && r.Form.Get("client_id") == "cid" {
			sawGrant.Store(true)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-cached",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/states/all", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-cached" {
			t.Errorf("expected bearer token, got %q", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(fixtureStates(t))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	source := newOAuthSource(t, server.URL, server.URL+"/token")
	if source.authMode != authOAuth {
		t.Fatalf("auth mode %q", source.authMode)
	}
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatalf("first sample: %v", err)
	}
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatalf("second sample: %v", err)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token should be cached, got %d fetches", tokenCalls.Load())
	}
	if !sawGrant.Load() {
		t.Fatal("token request must use grant_type=client_credentials")
	}
}

func TestOAuthRefreshBeforeExpiry(t *testing.T) {
	var tokenCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		n := tokenCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + strconv.Itoa(int(n)),
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/states/all", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixtureStates(t))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	source := newOAuthSource(t, server.URL, server.URL+"/token")
	base := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	now := base
	source.now = func() time.Time { return now }

	if _, err := source.sample(context.Background()); err != nil {
		t.Fatalf("first sample: %v", err)
	}
	now = base.Add(100 * time.Second)
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatalf("still-valid sample: %v", err)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token still valid, got %d fetches", tokenCalls.Load())
	}
	// expires_in 3600s, refreshMargin 45s → refresh once now+45s is no
	// longer strictly before expiry (t = 3556s).
	now = base.Add(3556 * time.Second)
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatalf("refresh sample: %v", err)
	}
	if tokenCalls.Load() != 2 {
		t.Fatalf("expected refresh before expiry, got %d fetches", tokenCalls.Load())
	}
}

func TestOAuthSingle401Retry(t *testing.T) {
	var tokenCalls atomic.Int32
	var apiCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		n := tokenCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "tok-" + strconv.Itoa(int(n)),
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/api/states/all", func(w http.ResponseWriter, r *http.Request) {
		n := apiCalls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok-2" {
			t.Errorf("retry must use the new token, got %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write(fixtureStates(t))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	source := newOAuthSource(t, server.URL, server.URL+"/token")
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatalf("401 retry should succeed: %v", err)
	}
	if tokenCalls.Load() != 2 {
		t.Fatalf("expected initial token + one refresh, got %d", tokenCalls.Load())
	}
	if apiCalls.Load() != 2 {
		t.Fatalf("expected two API calls, got %d", apiCalls.Load())
	}

	// A second consecutive 401 after the single retry is a hard failure.
	apiCalls.Store(0)
	mux2 := http.NewServeMux()
	var secondTokens atomic.Int32
	mux2.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		secondTokens.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "bad", "expires_in": 3600})
	})
	mux2.HandleFunc("/api/states/all", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	failServer := httptest.NewServer(mux2)
	defer failServer.Close()
	failing := newOAuthSource(t, failServer.URL, failServer.URL+"/token")
	_, err := failing.sample(context.Background())
	if err == nil {
		t.Fatal("expected invalid-credentials after a 401 retry")
	}
	if failing.StatusReason() != reasonOAuthInvalid {
		t.Fatalf("status reason %q", failing.StatusReason())
	}
}

func TestAnonFallbackWithoutCredentials(t *testing.T) {
	var tokenHits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		tokenHits.Add(1)
		w.WriteHeader(http.StatusTeapot)
	})
	mux.HandleFunc("/api/states/all", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("anon request must not send Authorization, got %q", got)
		}
		_, _ = w.Write(fixtureStates(t))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	source, err := New(config.Source{
		ID:          "anon",
		Type:        "aviation.opensky",
		Enabled:     true,
		PollSeconds: 1800,
		Broker:      server.URL,
	}, testLogger())
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	source.tokenURL = server.URL + "/token"
	if source.authMode != authAnon {
		t.Fatalf("expected anon, got %q", source.authMode)
	}
	if source.StatusReason() != authAnon {
		t.Fatalf("startup reason %q", source.StatusReason())
	}
	if _, err := source.sample(context.Background()); err != nil {
		t.Fatalf("anon sample: %v", err)
	}
	if tokenHits.Load() != 0 {
		t.Fatalf("anon mode must not hit the token endpoint")
	}
}

func TestCredentialsFileCamelAndSnake(t *testing.T) {
	dir := t.TempDir()
	camel := filepath.Join(dir, "camel.json")
	snake := filepath.Join(dir, "snake.json")
	if err := os.WriteFile(camel, []byte(`{"clientId":"c1","clientSecret":"s1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snake, []byte(`{"client_id":"c2","client_secret":"s2"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	id, secret, err := readCredentialsFile(camel)
	if err != nil || id != "c1" || secret != "s1" {
		t.Fatalf("camel: %q %q %v", id, secret, err)
	}
	id, secret, err = readCredentialsFile(snake)
	if err != nil || id != "c2" || secret != "s2" {
		t.Fatalf("snake: %q %q %v", id, secret, err)
	}
}

func TestOAuthPollFloorAllowsFasterInterval(t *testing.T) {
	_, err := New(config.Source{
		ID: "x", Type: "aviation.opensky", PollSeconds: 60,
		ClientID: "id", ClientSecret: "secret",
	}, testLogger())
	if err != nil {
		t.Fatalf("oauth should allow a 60s poll: %v", err)
	}
}

func TestRateLimitedHonoursRetryAfter(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/states/all", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	source, err := New(config.Source{
		ID: "rl", Type: "aviation.opensky", PollSeconds: 1800, Broker: server.URL,
	}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.sample(context.Background())
	limited, ok := err.(errRateLimited)
	if !ok {
		t.Fatalf("expected errRateLimited, got %T %v", err, err)
	}
	if limited.retryAfter != 90*time.Second {
		t.Fatalf("retry after %s", limited.retryAfter)
	}
	if source.StatusReason() != reasonRateLimited {
		t.Fatalf("reason %q", source.StatusReason())
	}
}

