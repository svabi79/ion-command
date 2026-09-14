// Package acled polls ACLED conflict events (https://acleddata.com/api/acled/read)
// for a recent look-back window. Access is operator-keyed: myACLED email
// and password (OAuth password grant) or a Bearer token in apiKey, both
// from gitignored local.json. The source refuses to start without
// credentials and does not ship a bundled dataset.
package acled

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
	"github.com/ion-command/ion-command/collector/internal/pollutil"
)

const (
	defaultBase     = "https://acleddata.com/api/acled/read"
	defaultTokenURL = "https://acleddata.com/oauth/token"
	pollDefault     = 15 * time.Minute
	pollFloor       = 10 * time.Minute
	lookBackDefault = 168 * time.Hour
	lookBackFloor   = 24 * time.Hour
	cacheName       = "events.json"
	maxEvents       = 200
	pageLimit       = 500
	refreshMargin   = 60 * time.Second
)

type oauthToken struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

type Source struct {
	id         string
	base       string
	tokenURL   string
	apiKey     string
	login      string
	password   string
	interval   time.Duration
	lookBack   time.Duration
	cache      pollutil.FileCache
	client     *http.Client
	logger     *slog.Logger
	now        func() time.Time
	fetch      func(ctx context.Context) ([]byte, error)
	tokenFetch func(ctx context.Context, refresh string) (oauthToken, error)

	mu    sync.Mutex
	token oauthToken
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "conflict.acled" {
		return nil, fmt.Errorf("unsupported acled source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	apiKey := strings.TrimSpace(sourceConfig.ApiKey)
	login := strings.TrimSpace(sourceConfig.Login)
	password := sourceConfig.Password
	if apiKey == "" && (login == "" || password == "") {
		return nil, fmt.Errorf("conflict.acled requires apiKey (Bearer token) or login+password (myACLED OAuth) in local.json")
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	if interval < pollFloor {
		return nil, fmt.Errorf("acled poll interval below ten minutes (got %s)", interval)
	}
	lookBack := lookBackDefault
	if sourceConfig.LookBackHours > 0 {
		lookBack = time.Duration(sourceConfig.LookBackHours * float64(time.Hour))
	}
	if lookBack < lookBackFloor {
		return nil, fmt.Errorf("acled lookBackHours below 24 hours (got %s)", lookBack)
	}
	base := defaultBase
	if sourceConfig.Broker != "" {
		base = sourceConfig.Broker
	}
	cacheDir := pollutil.ResolveCacheDir(sourceConfig.CacheDirectory, filepath.Join("data", "acled"))
	source := &Source{
		id: sourceConfig.ID, base: base, tokenURL: defaultTokenURL,
		apiKey: apiKey, login: login, password: password,
		interval: interval, lookBack: lookBack,
		cache:  pollutil.FileCache{Path: filepath.Join(cacheDir, cacheName)},
		client: &http.Client{Timeout: 60 * time.Second}, logger: logger, now: time.Now,
	}
	source.fetch = source.httpFetch
	source.tokenFetch = source.oauthToken
	return source, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "conflict.acled" }

func (s *Source) queryURL(now time.Time) string {
	from := now.Add(-s.lookBack).UTC().Format("2006-01-02")
	to := now.UTC().Format("2006-01-02")
	values := url.Values{}
	values.Set("_format", "json")
	values.Set("limit", strconv.Itoa(pageLimit))
	values.Set("event_date", from+"|"+to)
	values.Set("event_date_where", "BETWEEN")
	values.Set("fields", "event_id_cnty|event_date|event_type|sub_event_type|disorder_type|country|admin1|location|latitude|longitude|fatalities")
	return s.base + "?" + values.Encode()
}

func (s *Source) oauthToken(ctx context.Context, refresh string) (oauthToken, error) {
	form := url.Values{}
	form.Set("client_id", "acled")
	form.Set("scope", "authenticated")
	if refresh != "" {
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", refresh)
	} else {
		form.Set("grant_type", "password")
		form.Set("username", s.login)
		form.Set("password", s.password)
	}
	body, err := pollutil.Post(ctx, s.client, s.tokenURL, "application/x-www-form-urlencoded", []byte(form.Encode()), nil)
	if err != nil {
		return oauthToken{}, err
	}
	var response struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return oauthToken{}, fmt.Errorf("decode acled oauth token: %w", err)
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return oauthToken{}, fmt.Errorf("acled oauth token missing access_token")
	}
	expiry := 24 * time.Hour
	if response.ExpiresIn > 0 {
		expiry = time.Duration(response.ExpiresIn) * time.Second
	}
	return oauthToken{
		AccessToken:  response.AccessToken,
		RefreshToken: response.RefreshToken,
		Expiry:       s.now().Add(expiry),
	}, nil
}

func (s *Source) bearer(ctx context.Context) (string, error) {
	if s.login == "" {
		return s.apiKey, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token.AccessToken != "" && s.now().Before(s.token.Expiry.Add(-refreshMargin)) {
		return s.token.AccessToken, nil
	}
	token, err := s.tokenFetch(ctx, s.token.RefreshToken)
	if err != nil && s.token.RefreshToken != "" {
		token, err = s.tokenFetch(ctx, "")
	}
	if err != nil {
		return "", err
	}
	s.token = token
	return token.AccessToken, nil
}

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	token, err := s.bearer(ctx)
	if err != nil {
		return nil, err
	}
	body, err := pollutil.Get(ctx, s.client, s.queryURL(s.now()), map[string]string{
		"Authorization": "Bearer " + token,
		"Accept":        "application/json",
	})
	if err != nil {
		return nil, err
	}
	_ = s.cache.Store(body)
	return body, nil
}

func (s *Source) loadBody(ctx context.Context) ([]byte, error) {
	body, err := s.fetch(ctx)
	if err == nil {
		return body, nil
	}
	if cached, _, ok := s.cache.Load(); ok {
		s.logger.Warn("acled using disk cache", "source", s.id, "error", err)
		return cached, nil
	}
	return nil, err
}

type acledResponse struct {
	Status  json.RawMessage `json:"status"`
	Success bool            `json:"success"`
	Data    []acledEvent    `json:"data"`
}

type acledEvent struct {
	EventIDCnty  string          `json:"event_id_cnty"`
	EventDate    string          `json:"event_date"`
	EventType    string          `json:"event_type"`
	SubEventType string          `json:"sub_event_type"`
	DisorderType string          `json:"disorder_type"`
	Country      string          `json:"country"`
	Admin1       string          `json:"admin1"`
	Location     string          `json:"location"`
	Latitude     json.RawMessage `json:"latitude"`
	Longitude    json.RawMessage `json:"longitude"`
	Fatalities   json.RawMessage `json:"fatalities"`
}

func asFloat(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var asNumber float64
	if json.Unmarshal(raw, &asNumber) == nil {
		return asNumber, true
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		value, err := strconv.ParseFloat(strings.TrimSpace(asString), 64)
		return value, err == nil
	}
	return 0, false
}

func asInt(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var asNumber float64
	if json.Unmarshal(raw, &asNumber) == nil {
		return int(asNumber)
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		value, err := strconv.Atoi(strings.TrimSpace(asString))
		if err == nil {
			return value
		}
	}
	return 0
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.loadBody(ctx)
	if err != nil {
		return nil, err
	}
	var response acledResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode acled response: %w", err)
	}
	if !response.Success && len(response.Data) == 0 {
		return nil, fmt.Errorf("acled request unsuccessful")
	}
	now := s.now().UTC()
	records := make([]plugins.RawRecord, 0, len(response.Data))
	for _, event := range response.Data {
		id := strings.TrimSpace(event.EventIDCnty)
		if id == "" {
			continue
		}
		lat, latOK := asFloat(event.Latitude)
		lon, lonOK := asFloat(event.Longitude)
		if !latOK || !lonOK {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"kind":         "conflict",
			"eventId":      id,
			"eventDate":    event.EventDate,
			"eventType":    event.EventType,
			"subEventType": event.SubEventType,
			"disorderType": event.DisorderType,
			"country":      event.Country,
			"admin1":       event.Admin1,
			"location":     event.Location,
			"latitude":     lat,
			"longitude":    lon,
			"fatalities":   asInt(event.Fatalities),
			"attribution":  "Armed Conflict Location & Event Data Project (ACLED); www.acleddata.com",
			"provider":     "acled",
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID: "acled", SourceInstanceID: s.id,
			OriginalID:  "acled-" + id,
			Domain:      "conflict",
			ObservedUTC: now,
			Payload:     payload,
		})
		if len(records) >= maxEvents {
			break
		}
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("acled sample failed", "source", s.id, "error", err)
		} else if err == nil {
			s.logger.Info("acled snapshot", "source", s.id, "events", len(records))
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		wait := s.interval
		var limited pollutil.RateLimitedError
		if errors.As(err, &limited) {
			wait = max(wait, limited.RetryAfter)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
	}
}
