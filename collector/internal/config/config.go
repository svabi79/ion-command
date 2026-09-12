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
	// ClientSecret pairs with ClientID for OAuth2 client-credentials
	// sources (OpenSky). Never commit a real value; put it in the
	// gitignored local.json overlay.
	ClientSecret string `json:"clientSecret,omitempty"`
	// CredentialsFile is an optional path to a provider-supplied
	// credentials JSON (OpenSky's downloadable credentials.json). The
	// file may use clientId/clientSecret or client_id/client_secret.
	CredentialsFile string `json:"credentialsFile,omitempty"`
	PollSeconds     int     `json:"pollSeconds,omitempty"`
	// Login is the callsign or account used by sources that authenticate
	// (e.g. the RBN telnet feed).
	Login string `json:"login,omitempty"`
	// Password pairs with Login for sources that still use a password
	// (none of the shipped live sources; kept for operator overlays).
	Password string `json:"password,omitempty"`
	// CacheDirectory is a local on-disk cache for poll sources that
	// honour a tight provider budget (Launch Library) or keep a
	// removable third-party copy (submarine cables). Empty means the
	// source's own default under data/.
	CacheDirectory string `json:"cacheDirectory,omitempty"`
	// Filter is a server-side subscription filter for streaming sources that
	// support one (e.g. APRS-IS "filter" spec syntax, such as
	// "r/50.0/8.0/300"). Empty means the source's own sane default; the
	// sentinel "world" requests no filter at all.
	Filter string `json:"filter,omitempty"`
	// Geographic scope for area-query sources (e.g. ADS-B around a point).
	// orbital.celestrak reads the same pair as the ground station to compute
	// look angles and passes from; leaving it unset simply omits them.
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
	RadiusNm  float64 `json:"radiusNm,omitempty"`
	// AltitudeM is the observer's height above sea level, which matters for
	// look angles at low elevation. Zero is a fine default inland.
	AltitudeM float64 `json:"altitudeM,omitempty"`
	// RouteLookup toggles the callsign → origin/destination enrichment on
	// aviation.adsb sources (adsbdb.com). Unset means enabled.
	RouteLookup *bool `json:"routeLookup,omitempty"`
	// Bounding box (WGS84) for area-query sources that filter a rectangle
	// rather than a point + radius (e.g. NASA FIRMS fire detections).
	// BoxWest/BoxSouth is the lower-left corner, BoxEast/BoxNorth the
	// upper-right; the box does not wrap the antimeridian. All four zero
	// means "unset, use the source's default".
	BoxWest  float64 `json:"boxWest,omitempty"`
	BoxSouth float64 `json:"boxSouth,omitempty"`
	BoxEast  float64 `json:"boxEast,omitempty"`
	BoxNorth float64 `json:"boxNorth,omitempty"`
	// LookBackHours bounds how far back a snapshot-style source considers a
	// record current (e.g. FIRMS fire detections have no push/update model,
	// only a rolling window of recent satellite passes).
	LookBackHours float64 `json:"lookBackHours,omitempty"`
	// Satellite selects among several instruments/platforms a source can
	// poll (e.g. FIRMS: VIIRS_SNPP, VIIRS_NOAA20, VIIRS_NOAA21, MODIS).
	Satellite string `json:"satellite,omitempty"`
	// MapKey is a provider API key from configuration (e.g. NASA FIRMS
	// MAP_KEY). Never committed with a real value; sources that can also
	// work without one should treat this as optional.
	MapKey string `json:"mapKey,omitempty"`
	// ApiKey authenticates sources that require one (e.g. ais.aisstream).
	// Always supplied by the operator's own configuration file, never
	// committed.
	ApiKey string `json:"apiKey,omitempty"`
	// BoundingBoxes limits an area-subscription source (ais.aisstream) to the
	// regions the operator cares about. The provider requires at least one.
	BoundingBoxes []BoundingBox `json:"boundingBoxes,omitempty"`
}

// BoundingBox is a WGS84 latitude/longitude rectangle used by sources that
// accept more than one region of interest per subscription.
type BoundingBox struct {
	MinLatitude  float64 `json:"minLatitude"`
	MaxLatitude  float64 `json:"maxLatitude"`
	MinLongitude float64 `json:"minLongitude"`
	MaxLongitude float64 `json:"maxLongitude"`
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
	if err := applyLocalOverlay(&cfg, path); err != nil {
		return Config{}, err
	}
	return cfg, cfg.Validate()
}

// applyLocalOverlay merges secret-bearing fields from a sibling local.json
// onto matching source IDs. The overlay is gitignored; tracked configs
// (live.json, default.json) must never carry credentials. Loading
// local.json itself as the primary file is a no-op here.
func applyLocalOverlay(cfg *Config, configPath string) error {
	if strings.EqualFold(filepath.Base(configPath), "local.json") {
		return nil
	}
	overlayPath := filepath.Join(filepath.Dir(configPath), "local.json")
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read local overlay: %w", err)
	}
	var overlay Config
	if err := json.Unmarshal(data, &overlay); err != nil {
		return fmt.Errorf("decode local overlay: %w", err)
	}
	byID := make(map[string]int, len(cfg.Sources))
	for i, source := range cfg.Sources {
		byID[source.ID] = i
	}
	for _, extra := range overlay.Sources {
		index, ok := byID[extra.ID]
		if !ok {
			continue
		}
		dst := &cfg.Sources[index]
		if extra.ClientID != "" {
			dst.ClientID = extra.ClientID
		}
		if extra.ClientSecret != "" {
			dst.ClientSecret = extra.ClientSecret
		}
		if extra.CredentialsFile != "" {
			dst.CredentialsFile = extra.CredentialsFile
		}
		if extra.ApiKey != "" {
			dst.ApiKey = extra.ApiKey
		}
		if extra.MapKey != "" {
			dst.MapKey = extra.MapKey
		}
		if extra.Password != "" {
			dst.Password = extra.Password
		}
		if extra.Login != "" {
			dst.Login = extra.Login
		}
		if extra.PollSeconds > 0 {
			dst.PollSeconds = extra.PollSeconds
		}
	}
	return nil
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
