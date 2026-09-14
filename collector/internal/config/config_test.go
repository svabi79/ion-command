package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A configured source must not inherit fields from the default source at the
// same slice index (json.Unmarshal merges into existing slice elements).
func TestLoadReplacesDefaultSourcesCompletely(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
  "server": { "listenAddress": "127.0.0.1:7810", "writeTimeoutSeconds": 10 },
  "pipeline": { "queueCapacity": 16, "clientQueueCapacity": 16, "workerCount": 1 },
  "recording": { "enabled": false, "directory": "data", "flushIntervalSeconds": 1 },
  "sources": [ { "id": "swpc", "type": "spaceweather.swpc", "enabled": true } ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.Sources) != 1 {
		t.Fatalf("expected exactly one source, got %d", len(cfg.Sources))
	}
	source := cfg.Sources[0]
	if source.EventsPerSecond != 0 || source.Seed != 0 {
		t.Fatalf("source inherited default fields: %+v", source)
	}
}

func TestLoadMergesLocalOverlaySecrets(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live.json")
	overlay := filepath.Join(dir, "local.json")
	liveBody := `{
  "server": { "listenAddress": "127.0.0.1:7810", "writeTimeoutSeconds": 10 },
  "pipeline": { "queueCapacity": 16, "clientQueueCapacity": 16, "workerCount": 1 },
  "recording": { "enabled": false, "directory": "data", "flushIntervalSeconds": 1 },
  "sources": [
    { "id": "opensky-world", "type": "aviation.opensky", "enabled": true, "pollSeconds": 1800 },
    { "id": "openaq-example", "type": "weather.openaq", "enabled": false }
  ]
}`
	overlayBody := `{
  "sources": [
    { "id": "opensky-world", "clientId": "cid", "clientSecret": "csecret", "pollSeconds": 60 },
    { "id": "openaq-example", "apiKey": "aq-key", "enabled": true }
  ]
}`
	if err := os.WriteFile(live, []byte(liveBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(overlay, []byte(overlayBody), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(live)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Sources[0].ClientID != "cid" || cfg.Sources[0].ClientSecret != "csecret" {
		t.Fatalf("opensky overlay not applied: %+v", cfg.Sources[0])
	}
	if cfg.Sources[0].PollSeconds != 60 {
		t.Fatalf("poll overlay %d", cfg.Sources[0].PollSeconds)
	}
	if cfg.Sources[1].ApiKey != "aq-key" {
		t.Fatalf("openaq overlay not applied: %+v", cfg.Sources[1])
	}
	if !cfg.Sources[1].Enabled {
		t.Fatal("overlay enabled:true must turn a shipped-disabled source on")
	}
}
