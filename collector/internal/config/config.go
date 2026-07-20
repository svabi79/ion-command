package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// RetainLatest lists semantic types whose latest observation per entity
	// is replayed to every newly connected live client, so state-like values
	// (space weather, soundings) do not stay blank until the next sample.
	RetainLatest []string `json:"retainLatest"`
}

type Recording struct {
	Enabled              bool   `json:"enabled"`
	Directory            string `json:"directory"`
	FlushIntervalSeconds int    `json:"flushIntervalSeconds"`
	// MaxTotalGigabytes caps the recording directory: on every hourly
	// rotation the oldest files are deleted until the total is back under
	// the limit. Zero keeps the previous unbounded behavior.
	MaxTotalGigabytes float64 `json:"maxTotalGigabytes"`
}

type Source struct {
	ID              string  `json:"id"`
	Type            string  `json:"type"`
	Enabled         bool    `json:"enabled"`
	EventsPerSecond float64 `json:"eventsPerSecond,omitempty"`
	Seed            int64   `json:"seed,omitempty"`
	Broker          string  `json:"broker,omitempty"`
	Topic           string  `json:"topic,omitempty"`
	ClientID        string  `json:"clientId,omitempty"`
	PollSeconds     int     `json:"pollSeconds,omitempty"`
	// Login is the callsign or account used by sources that authenticate
	// (e.g. the RBN telnet feed, or an OpenSky account).
	Login string `json:"login,omitempty"`
	// Password pairs with Login for HTTP basic-auth sources (OpenSky).
	Password string `json:"password,omitempty"`
	// Geographic scope for area-query sources (e.g. ADS-B around a point).
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
	RadiusNm  float64 `json:"radiusNm,omitempty"`
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
	// json.Unmarshal merges into existing slice ELEMENTS, so a configured
	// source would silently inherit fields of the default source at the same
	// index. A config file therefore always defines the complete source list.
	cfg.Sources = nil
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
		if source.Enabled && strings.HasPrefix(source.Type, "mock.") && source.EventsPerSecond <= 0 {
			return fmt.Errorf("sources[%d].eventsPerSecond must be positive", i)
		}
	}
	return nil
}
