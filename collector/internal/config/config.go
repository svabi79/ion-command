package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Branding struct {
	ProductName string `json:"productName"`
	Subtitle    string `json:"subtitle"`
}

type Server struct {
	ListenAddress       string `json:"listenAddress"`
	WriteTimeoutSeconds int    `json:"writeTimeoutSeconds"`
}

type Pipeline struct {
	QueueCapacity       int `json:"queueCapacity"`
	ClientQueueCapacity int `json:"clientQueueCapacity"`
	WorkerCount         int `json:"workerCount"`
}

type Recording struct {
	Enabled              bool   `json:"enabled"`
	Directory            string `json:"directory"`
	FlushIntervalSeconds int    `json:"flushIntervalSeconds"`
}

type Source struct {
	ID              string  `json:"id"`
	Type            string  `json:"type"`
	Enabled         bool    `json:"enabled"`
	EventsPerSecond float64 `json:"eventsPerSecond"`
	Seed            int64   `json:"seed"`
}

type Config struct {
	Branding  Branding  `json:"branding"`
	Server    Server    `json:"server"`
	Pipeline  Pipeline  `json:"pipeline"`
	Recording Recording `json:"recording"`
	Sources   []Source  `json:"sources"`
}

func Default() Config {
	return Config{
		Branding:  Branding{ProductName: "ION COMMAND", Subtitle: "Global Geospatial Operations & HF Propagation Command Center"},
		Server:    Server{ListenAddress: "127.0.0.1:7810", WriteTimeoutSeconds: 10},
		Pipeline:  Pipeline{QueueCapacity: 16384, ClientQueueCapacity: 4096, WorkerCount: 4},
		Recording: Recording{Enabled: true, Directory: "data/recordings", FlushIntervalSeconds: 1},
		Sources: []Source{
			{ID: "mock-radio-primary", Type: "mock.radio", Enabled: true, EventsPerSecond: 40, Seed: 7810},
			{ID: "mock-lightning-primary", Type: "mock.lightning", Enabled: true, EventsPerSecond: 2, Seed: 7811},
			{ID: "mock-spaceweather-primary", Type: "mock.spaceweather", Enabled: true, EventsPerSecond: 0.1, Seed: 7812},
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, cfg.Validate()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if !filepath.IsAbs(cfg.Recording.Directory) {
		cfg.Recording.Directory = filepath.Clean(filepath.Join(filepath.Dir(path), cfg.Recording.Directory))
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if c.Server.ListenAddress == "" {
		return fmt.Errorf("server.listenAddress is required")
	}
	if c.Pipeline.QueueCapacity < 1 || c.Pipeline.ClientQueueCapacity < 1 || c.Pipeline.WorkerCount < 1 {
		return fmt.Errorf("pipeline capacities and workerCount must be positive")
	}
	for i, source := range c.Sources {
		if source.ID == "" || source.Type == "" {
			return fmt.Errorf("sources[%d] requires id and type", i)
		}
		if source.Enabled && source.EventsPerSecond <= 0 {
			return fmt.Errorf("sources[%d].eventsPerSecond must be positive", i)
		}
	}
	return nil
}
