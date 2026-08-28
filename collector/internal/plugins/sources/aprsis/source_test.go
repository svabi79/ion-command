package aprsis

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

func TestLoginRequired(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "aprs.is"}, slog.Default()); err == nil {
		t.Fatal("missing login callsign must be rejected")
	}
}

func TestWrongTypeRejected(t *testing.T) {
	if _, err := New(config.Source{ID: "x", Type: "aprs.other", Login: "N0CALL"}, slog.Default()); err == nil {
		t.Fatal("wrong source type must be rejected")
	}
}

func TestBuildFilterDefault(t *testing.T) {
	filter := buildFilter(config.Source{})
	if filter != defaultFilter {
		t.Fatalf("expected illustrative default filter, got %q", filter)
	}
}

func TestBuildFilterWorldSentinelDisablesFilter(t *testing.T) {
	if filter := buildFilter(config.Source{Filter: "world"}); filter != "" {
		t.Fatalf("expected empty filter for world sentinel, got %q", filter)
	}
}

func TestBuildFilterExplicitWins(t *testing.T) {
	filter := buildFilter(config.Source{Filter: "r/1/2/3", Latitude: 47, Longitude: 8})
	if filter != "r/1/2/3" {
		t.Fatalf("explicit filter should win over lat/lon, got %q", filter)
	}
}

func TestBuildFilterFromCoordinates(t *testing.T) {
	filter := buildFilter(config.Source{Latitude: 47.3, Longitude: 8.5, RadiusNm: 100})
	want := "r/47.3/8.5/185.2"
	if filter != want {
		t.Fatalf("expected %q, got %q", want, filter)
	}
}

func TestBuildFilterFromCoordinatesDefaultRadius(t *testing.T) {
	filter := buildFilter(config.Source{Latitude: 47.3, Longitude: 8.5})
	want := "r/47.3/8.5/200"
	if filter != want {
		t.Fatalf("expected default 200km radius, got %q", filter)
	}
}

func TestWrapLineNeverInspectsContent(t *testing.T) {
	record, ok := wrapLine(`N0CALL>APRS,TCPIP*:!4903.50N/07201.75W-Test`, "inst", 42)
	if !ok {
		t.Fatal("expected a record")
	}
	if record.SourcePluginID != "aprsis" || record.Domain != "aprs" {
		t.Fatalf("unexpected record shape: %+v", record)
	}
	var payload struct {
		Raw string `json:"raw"`
	}
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Raw != `N0CALL>APRS,TCPIP*:!4903.50N/07201.75W-Test` {
		t.Fatalf("payload should carry the raw line verbatim, got %q", payload.Raw)
	}
}

// TestStreamLoginAndForwarding runs a minimal fake APRS-IS server over a
// real TCP loopback connection and checks that: the login line carries the
// documented read-only passcode and the configured filter, banner/keepalive
// "#" lines never become records, and a plain packet line is forwarded
// completely unexamined (the raw text survives byte for byte).
func TestStreamLoginAndForwarding(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	const packetLine = "N0CALL>APRS,TCPIP*:!4903.50N/07201.75W-Test 001234"
	loginReceived := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		login, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		loginReceived <- strings.TrimRight(login, "\r\n")
		conn.Write([]byte("# javAPRSSrvr 4.1.6\r\n"))
		conn.Write([]byte("# logresp N0CALL unverified, server TEST\r\n"))
		conn.Write([]byte(packetLine + "\r\n"))
		conn.Write([]byte("# aliveness keepalive\r\n"))
		// Keep the connection open briefly so the client's read loop has
		// time to deliver the packet before the test cancels the context.
		time.Sleep(300 * time.Millisecond)
	}()

	source, err := New(config.Source{ID: "test", Type: "aprs.is", Login: "n0call", Filter: "r/1/2/3"}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	source.address = listener.Addr().String()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	output := make(chan plugins.RawRecord, 4)
	done := make(chan error, 1)
	go func() { done <- source.stream(ctx, output) }()

	select {
	case login := <-loginReceived:
		if !strings.Contains(login, "user N0CALL") || !strings.Contains(login, "pass -1") || !strings.Contains(login, "filter r/1/2/3") {
			t.Fatalf("unexpected login line: %q", login)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server never received a login line")
	}

	select {
	case record := <-output:
		var payload struct {
			Raw string `json:"raw"`
		}
		if err := json.Unmarshal(record.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Raw != packetLine {
			t.Fatalf("expected the packet line forwarded verbatim, got %q", payload.Raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("packet line was never forwarded")
	}
	cancel()
	<-done
}
