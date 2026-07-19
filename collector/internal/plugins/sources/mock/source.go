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
	case "mock.radio", "mock.lightning", "mock.spaceweather", "mock.ionosonde":
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
	case "mock.ionosonde":
		domain = "ionosphere"
		payload = ionosondePayload(rng, sequence)
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
	// A weighted spread of real ADIF DXCC entity codes so region instruments
	// have realistic data (Germany, Italy, USA, Japan, Spain, England, France,
	// Switzerland, Poland, Brazil).
	dxcc := []int{230, 230, 248, 291, 291, 291, 339, 281, 223, 227, 287, 269, 108}
	fromLon, fromLat := randomLandishPoint(rng)
	toLon, toLat := randomLandishPoint(rng)
	return map[string]any{
		"txDxcc": dxcc[rng.IntN(len(dxcc))],
		"rxDxcc": dxcc[rng.IntN(len(dxcc))],
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

// ionosondePayload cycles through a fixed set of plausible sounder sites so
// path-analysis instruments can be exercised without the live feed.
func ionosondePayload(rng *rand.Rand, sequence uint64) map[string]any {
	stations := []struct {
		id   string
		name string
		lat  float64
		lon  float64
	}{
		{"MO155", "Moscow", 55.5, 37.3},
		{"EA036", "El Arenosillo", 37.1, -6.7},
		{"BC840", "Boulder", 40.0, -105.3},
		{"WP937", "Wallops Is", 37.9, -75.5},
		{"GA762", "Gakona", 62.4, -145.0},
		{"JJ433", "Kokubunji", 35.7, 139.5},
		{"CB53N", "Cocos Is", -12.2, 96.8},
		{"GR13L", "Grahamstown", -33.3, 26.5},
	}
	station := stations[sequence%uint64(len(stations))]
	foF2 := 4.0 + rng.Float64()*8.0
	m3000 := 2.6 + rng.Float64()*0.9
	return map[string]any{
		"stationId":  station.id,
		"name":       station.name,
		"latitude":   station.lat,
		"longitude":  station.lon,
		"foF2Mhz":    math.Round(foF2*10) / 10,
		"mufdMhz":    math.Round(foF2*m3000*10) / 10,
		"hmF2Km":     220 + rng.IntN(140),
		"foEMhz":     math.Round((2+rng.Float64()*2)*10) / 10,
		"m3000":      math.Round(m3000*100) / 100,
		"tec":        math.Round((5+rng.Float64()*30)*10) / 10,
		"confidence": 100,
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
