package acled

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
)

func TestRequiresCredentials(t *testing.T) {
	if _, err := New(config.Source{ID: "a", Type: "conflict.acled"}, slog.Default()); err == nil {
		t.Fatal("expected fail-closed without credentials")
	}
	if _, err := New(config.Source{ID: "a", Type: "conflict.acled", Login: "user@example.com"}, slog.Default()); err == nil {
		t.Fatal("expected fail-closed with login only")
	}
}

func TestPollFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "a", Type: "conflict.acled", ApiKey: "tok", PollSeconds: 60}, slog.Default()); err == nil {
		t.Fatal("expected floor rejection")
	}
}

func TestLookBackFloor(t *testing.T) {
	if _, err := New(config.Source{ID: "a", Type: "conflict.acled", ApiKey: "tok", LookBackHours: 1}, slog.Default()); err == nil {
		t.Fatal("expected look-back floor rejection")
	}
}

func TestSampleFromFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/events.json")
	if err != nil {
		t.Fatal(err)
	}
	source, err := New(config.Source{ID: "a", Type: "conflict.acled", ApiKey: "test-token", PollSeconds: 900}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.fetch = func(context.Context) ([]byte, error) { return body, nil }
	records, err := source.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
	var first map[string]any
	if err := json.Unmarshal(records[0].Payload, &first); err != nil {
		t.Fatal(err)
	}
	if records[0].OriginalID != "acled-IRQ1234" || first["eventType"] != "Battles" {
		t.Fatalf("%s %v", records[0].OriginalID, first)
	}
	if first["fatalities"].(float64) != 3 || first["latitude"].(float64) != 33.3152 {
		t.Fatalf("numeric fields %v", first)
	}
	if first["kind"] != "conflict" || records[0].Domain != "conflict" {
		t.Fatalf("%v", first)
	}
}

func TestBearerTokenAndOAuth(t *testing.T) {
	var sawAuth string
	var grants []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			body, _ := io.ReadAll(r.Body)
			grants = append(grants, string(body))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"live-token","refresh_token":"r1","expires_in":3600,"token_type":"Bearer"}`))
			return
		}
		sawAuth = r.Header.Get("Authorization")
		fixture, _ := os.ReadFile("testdata/events.json")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(server.Close)

	keyed, err := New(config.Source{ID: "k", Type: "conflict.acled", ApiKey: "static-token", Broker: server.URL + "/api/acled/read"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	keyed.client = server.Client()
	if _, err := keyed.sample(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sawAuth != "Bearer static-token" {
		t.Fatalf("static token auth %q", sawAuth)
	}

	oauth, err := New(config.Source{
		ID: "o", Type: "conflict.acled", Login: "user@example.com", Password: "secret",
		Broker: server.URL + "/api/acled/read",
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	oauth.tokenURL = server.URL + "/oauth/token"
	oauth.client = server.Client()
	oauth.now = func() time.Time { return time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC) }
	records, err := oauth.sample(context.Background())
	if err != nil || len(records) != 2 {
		t.Fatalf("%v %d", err, len(records))
	}
	if sawAuth != "Bearer live-token" {
		t.Fatalf("oauth auth %q", sawAuth)
	}
	if len(grants) != 1 || !strings.Contains(grants[0], "grant_type=password") {
		t.Fatalf("grants %v", grants)
	}
}
