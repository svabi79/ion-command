package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

type Source struct {
	config config.Source
}

func New(sourceConfig config.Source) (*Source, error) {
	if sourceConfig.EventsPerSecond <= 0 {
		return nil, fmt.Errorf("eventsPerSecond must be positive")
	}
	switch sourceConfig.Type {
	case "mock.radio", "mock.lightning", "mock.spaceweather":
	default:
		return nil, fmt.Errorf("unsupported mock source type %q", sourceConfig.Type)
	}
	return &Source{config: sourceConfig}, nil
}

func (s *Source) ID() string   { return s.config.ID }
func (s *Source) Type() string { return s.config.Type }

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	period := time.Duration(float64(time.Second) / s.config.EventsPerSecond)
	if period < time.Millisecond {
		period = time.Millisecond
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	seed := uint64(s.config.Seed)
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	sequence := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return nil
		case observed := <-ticker.C:
			sequence++
			record, err := s.generate(rng, sequence, observed.UTC())
			if err != nil {
				return err
			}
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

func (s *Source) generate(rng *rand.Rand, sequence uint64, observed time.Time) (plugins.RawRecord, error) {
	var domain string
	var payload any
	switch s.config.Type {
	case "mock.radio":
		domain = "hamradio"
		payload = radioPayload(rng, sequence)
	case "mock.lightning":
		domain = "weather"
		payload = lightningPayload(rng, sequence)
	case "mock.spaceweather":
		domain = "spaceweather"
		payload = spaceWeatherPayload(rng, sequence)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return plugins.RawRecord{}, fmt.Errorf("encode mock payload: %w", err)
	}
	return plugins.RawRecord{
		SourcePluginID:   s.config.Type,
		SourceInstanceID: s.config.ID,
		OriginalID:       fmt.Sprintf("%s-%d", s.config.ID, sequence),
		Domain:           domain,
		ObservedUTC:      observed,
		Payload:          encoded,
	}, nil
}

func radioPayload(rng *rand.Rand, sequence uint64) map[string]any {
	bands := []struct {
		name      string
		frequency int64
		mode      string
	}{
		{"80m", 3_573_000, "FT8"}, {"40m", 7_074_000, "FT8"}, {"30m", 10_136_000, "FT8"},
		{"20m", 14_074_000, "FT8"}, {"17m", 18_100_000, "FT8"}, {"15m", 21_074_000, "FT8"},
		{"10m", 28_074_000, "FT8"}, {"6m", 50_313_000, "FT8"},
	}
	band := bands[rng.IntN(len(bands))]
	fromLon, fromLat := randomLandishPoint(rng)
	toLon, toLat := randomLandishPoint(rng)
	return map[string]any{
		"spotId":      fmt.Sprintf("spot-%d", sequence),
		"txCallsign":  fmt.Sprintf("TX%05d", sequence%100000),
		"rxCallsign":  fmt.Sprintf("RX%05d", (sequence*37)%100000),
		"txLongitude": fromLon,
		"txLatitude":  fromLat,
		"rxLongitude": toLon,
		"rxLatitude":  toLat,
		"frequencyHz": band.frequency,
		"band":        band.name,
		"mode":        band.mode,
		"snrDb":       -28 + rng.IntN(44),
	}
}

func lightningPayload(rng *rand.Rand, sequence uint64) map[string]any {
	lon, lat := randomLandishPoint(rng)
	return map[string]any{
		"strikeId":      fmt.Sprintf("strike-%d", sequence),
		"longitude":     lon,
		"latitude":      lat,
		"peakCurrentKa": math.Round((-100+rng.Float64()*200)*10) / 10,
	}
}

func spaceWeatherPayload(rng *rand.Rand, sequence uint64) map[string]any {
	return map[string]any{
		"sampleId":          fmt.Sprintf("sw-%d", sequence),
		"kp":                math.Round((1+rng.Float64()*5)*10) / 10,
		"aIndex":            4 + rng.IntN(30),
		"solarFlux":         80 + rng.IntN(180),
		"solarWindSpeedKms": 280 + rng.IntN(520),
		"solarWindDensity":  math.Round((1+rng.Float64()*11)*10) / 10,
		"imfBzNt":           math.Round((-10+rng.Float64()*20)*10) / 10,
	}
}

func randomLandishPoint(rng *rand.Rand) (float64, float64) {
	// Uniform enough for visual load testing while avoiding polar clustering.
	return -180 + rng.Float64()*360, -65 + rng.Float64()*130
}
