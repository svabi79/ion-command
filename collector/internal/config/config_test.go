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
