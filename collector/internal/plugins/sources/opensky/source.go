// Package opensky polls the OpenSky Network global state snapshot
// (https://opensky-network.org/api/states/all). One request returns every
// tracked aircraft worldwide, which complements the fast regional adsb.lol
// point queries with planet-wide ambient coverage.
//
// OpenSky retired HTTP basic auth. This source uses OAuth2 client
// credentials when clientId/clientSecret (or a credentials.json path) are
// present, and otherwise keeps running anonymously with the smaller daily
// credit allowance.
package opensky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ion-command/ion-command/collector/internal/config"
	"github.com/ion-command/ion-command/collector/internal/plugins"
)

const (
	defaultBase     = "https://opensky-network.org"
	defaultTokenURL = "https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token"
	pollDefault     = 15 * time.Minute
	pollFloorAnon   = 5 * time.Minute
	pollFloorOAuth  = 10 * time.Second
	backoffInitial  = 2 * time.Minute
	backoffMax      = 30 * time.Minute
	refreshMargin   = 45 * time.Second
	// Anonymous access is credit-limited; authenticated clients get a
	// much larger daily budget. /states/all is treated as one request.
	anonDailyRequests  = 100
	oauthDailyRequests = 1000
	// A state vector counts as live when its position fix is at most this
	// old at fetch time; the snapshot carries hours-stale entries too.
	positionMaxAge = 120 * time.Second

	reasonOAuthInvalid = "oauth_invalid_credentials"
	reasonRateLimited  = "rate_limited"
	reasonBudget       = "credit_budget_exhausted"
	authOAuth          = "oauth"
	authAnon           = "anon"
)

// kindForCategory maps the OpenSky aircraft category enum to the generic
// airframe kind vocabulary shared with the readsb-based sources.
func kindForCategory(category int) string {
	switch category {
	case 8:
		return "helicopter"
	case 9, 12:
		return "glider"
	case 10:
		return "balloon"
	case 14:
		return "drone"
	default:
		return "aircraft"
	}
}

type oauthToken struct {
	AccessToken string
	Expiry      time.Time
}

type Source struct {
	id           string
	base         string
	tokenURL     string
	clientID     string
	clientSecret string
	authMode     string
	interval     time.Duration
	client       *http.Client
	logger       *slog.Logger
	// fetch is swappable for tests that only exercise snapshot parsing.
	fetch func(ctx context.Context) ([]byte, error)
	now   func() time.Time

	mu           sync.Mutex
	token        oauthToken
	reason       string
	dayUTC       string
	usedRequests int
	did401Retry  bool
}

func New(sourceConfig config.Source, logger *slog.Logger) (*Source, error) {
	if sourceConfig.Type != "aviation.opensky" {
		return nil, fmt.Errorf("unsupported opensky source type %q", sourceConfig.Type)
	}
	if logger == nil {
		logger = slog.Default()
	}
	clientID, clientSecret, err := resolveCredentials(sourceConfig)
	if err != nil {
		return nil, err
	}
	authMode := authAnon
	if clientID != "" && clientSecret != "" {
		authMode = authOAuth
	}
	if sourceConfig.Login != "" || sourceConfig.Password != "" {
		logger.Warn("opensky basic auth is retired; ignoring login/password", "source", sourceConfig.ID, "authMode", authMode)
	}
	interval := pollDefault
	if sourceConfig.PollSeconds > 0 {
		interval = time.Duration(sourceConfig.PollSeconds) * time.Second
	}
	floor := pollFloorAnon
	if authMode == authOAuth {
		floor = pollFloorOAuth
	}
	if interval < floor {
		return nil, fmt.Errorf("opensky poll interval below %s would exhaust the %s credit budget (got %s)", floor, authMode, interval)
	}
	source := &Source{
		id:           sourceConfig.ID,
		base:         defaultBase,
		tokenURL:     defaultTokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		authMode:     authMode,
		interval:     interval,
		client:       &http.Client{Timeout: 90 * time.Second},
		logger:       logger,
		now:          time.Now,
		reason:       authMode,
	}
	if sourceConfig.Broker != "" {
		source.base = strings.TrimSuffix(sourceConfig.Broker, "/")
	}
	source.fetch = source.httpFetch
	logger.Info("opensky auth mode", "source", source.id, "authMode", authMode)
	return source, nil
}

func resolveCredentials(sourceConfig config.Source) (string, string, error) {
	clientID := strings.TrimSpace(sourceConfig.ClientID)
	clientSecret := strings.TrimSpace(sourceConfig.ClientSecret)
	if sourceConfig.CredentialsFile == "" {
		return clientID, clientSecret, nil
	}
	fileID, fileSecret, err := readCredentialsFile(sourceConfig.CredentialsFile)
	if err != nil {
		return "", "", fmt.Errorf("opensky credentials file: %w", err)
	}
	if clientID == "" {
		clientID = fileID
	}
	if clientSecret == "" {
		clientSecret = fileSecret
	}
	return clientID, clientSecret, nil
}

type credentialsFile struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	ClientIDAlt  string `json:"client_id"`
	SecretAlt    string `json:"client_secret"`
}

func readCredentialsFile(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var parsed credentialsFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", "", err
	}
	clientID := strings.TrimSpace(parsed.ClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(parsed.ClientIDAlt)
	}
	secret := strings.TrimSpace(parsed.ClientSecret)
	if secret == "" {
		secret = strings.TrimSpace(parsed.SecretAlt)
	}
	return clientID, secret, nil
}

func (s *Source) ID() string   { return s.id }
func (s *Source) Type() string { return "aviation.opensky" }

func (s *Source) StatusReason() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reason == "" {
		return s.authMode
	}
	return s.reason
}

func (s *Source) setReason(reason string) {
	s.mu.Lock()
	s.reason = reason
	s.mu.Unlock()
}

type errRateLimited struct{ retryAfter time.Duration }

func (e errRateLimited) Error() string {
	return fmt.Sprintf("rate limited by opensky (retry after %s)", e.retryAfter)
}

type errInvalidCredentials struct{}

func (errInvalidCredentials) Error() string { return reasonOAuthInvalid }

func (s *Source) dailyLimit() int {
	if s.authMode == authOAuth {
		return oauthDailyRequests
	}
	return anonDailyRequests
}

func (s *Source) consumeCredit() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	day := s.now().UTC().Format("2006-01-02")
	if s.dayUTC != day {
		s.dayUTC = day
		s.usedRequests = 0
	}
	if s.usedRequests >= s.dailyLimit() {
		s.reason = reasonBudget
		midnight := s.now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
		return errRateLimited{retryAfter: midnight.Sub(s.now().UTC())}
	}
	s.usedRequests++
	return nil
}

func (s *Source) tokenValidLocked(at time.Time) bool {
	return s.token.AccessToken != "" && at.Add(refreshMargin).Before(s.token.Expiry)
}

func (s *Source) fetchToken(ctx context.Context) error {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.clientID)
	form.Set("client_secret", s.clientSecret)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		s.setReason(reasonOAuthInvalid)
		return errInvalidCredentials{}
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("opensky token endpoint returned %s", response.Status)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("decode opensky token: %w", err)
	}
	if parsed.AccessToken == "" {
		return fmt.Errorf("opensky token response missing access_token")
	}
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 1800
	}
	s.mu.Lock()
	s.token = oauthToken{
		AccessToken: parsed.AccessToken,
		Expiry:      s.now().Add(time.Duration(parsed.ExpiresIn) * time.Second),
	}
	if s.reason == reasonOAuthInvalid {
		s.reason = s.authMode
	}
	s.mu.Unlock()
	return nil
}

func (s *Source) ensureToken(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.tokenValidLocked(s.now()) {
		token := s.token.AccessToken
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()
	if err := s.fetchToken(ctx); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token.AccessToken, nil
}

func parseRetryAfter(response *http.Response, fallback time.Duration) time.Duration {
	if header := response.Header.Get("Retry-After"); header != "" {
		if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		if when, err := http.ParseTime(header); err == nil {
			if wait := time.Until(when); wait > 0 {
				return wait
			}
		}
	}
	if header := response.Header.Get("X-Rate-Limit-Retry-After-Seconds"); header != "" {
		if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return fallback
}

func (s *Source) doStatesRequest(ctx context.Context, token string) (*http.Response, error) {
	apiURL := s.base + "/api/states/all?extended=1"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return s.client.Do(request)
}

func (s *Source) httpFetch(ctx context.Context) ([]byte, error) {
	if err := s.consumeCredit(); err != nil {
		return nil, err
	}
	var token string
	if s.authMode == authOAuth {
		var err error
		token, err = s.ensureToken(ctx)
		if err != nil {
			return nil, err
		}
	}
	response, err := s.doStatesRequest(ctx, token)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusUnauthorized && s.authMode == authOAuth {
		response.Body.Close()
		s.mu.Lock()
		already := s.did401Retry
		s.token = oauthToken{}
		s.did401Retry = true
		s.mu.Unlock()
		if already {
			s.setReason(reasonOAuthInvalid)
			return nil, errInvalidCredentials{}
		}
		if err := s.fetchToken(ctx); err != nil {
			return nil, err
		}
		s.mu.Lock()
		retryToken := s.token.AccessToken
		s.mu.Unlock()
		response, err = s.doStatesRequest(ctx, retryToken)
		if err != nil {
			return nil, err
		}
		if response.StatusCode == http.StatusUnauthorized {
			response.Body.Close()
			s.setReason(reasonOAuthInvalid)
			return nil, errInvalidCredentials{}
		}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests || response.StatusCode == 420 {
		s.setReason(reasonRateLimited)
		return nil, errRateLimited{retryAfter: parseRetryAfter(response, backoffInitial)}
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s/api/states/all returned %s", s.base, response.Status)
	}
	s.mu.Lock()
	s.did401Retry = false
	if s.reason == reasonRateLimited || s.reason == reasonBudget || s.reason == reasonOAuthInvalid {
		s.reason = s.authMode
	}
	s.mu.Unlock()
	return io.ReadAll(io.LimitReader(response.Body, 64<<20))
}

type statesResponse struct {
	Time   int64               `json:"time"`
	States [][]json.RawMessage `json:"states"`
}

func rawString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func rawFloat(raw json.RawMessage) (float64, bool) {
	// A JSON null unmarshals into a float64 as a no-op (no error, value stays
	// 0), which would masquerade as a real zero. Treat null/empty as missing
	// so altitude/time fallbacks actually trigger (audit finding #12).
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var value float64
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	return 0, false
}

func rawBool(raw json.RawMessage) bool {
	var value bool
	_ = json.Unmarshal(raw, &value)
	return value
}

func (s *Source) sample(ctx context.Context) ([]plugins.RawRecord, error) {
	body, err := s.fetch(ctx)
	if err != nil {
		return nil, err
	}
	var response statesResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode opensky response: %w", err)
	}
	now := s.now().UTC()
	snapshot := response.Time
	// A missing snapshot time would make snapshot-positionTime hugely negative
	// and pass every stale vector through the age gate (finding #16).
	if snapshot == 0 {
		snapshot = now.Unix()
	}
	// Markers must outlive the slow poll cycle plus request jitter.
	validSeconds := int(s.interval/time.Second) + 180
	records := make([]plugins.RawRecord, 0, len(response.States))
	for _, state := range response.States {
		// State vector layout (extended=1): 0 icao24, 1 callsign,
		// 2 origin_country, 3 time_position, 4 last_contact, 5 lon, 6 lat,
		// 7 baro_altitude m, 8 on_ground, 9 velocity m/s, 10 true_track,
		// 11 vertical_rate m/s, 13 geo_altitude, 14 squawk, 17 category.
		if len(state) < 17 {
			continue
		}
		hex := rawString(state[0])
		lon, lonOK := rawFloat(state[5])
		lat, latOK := rawFloat(state[6])
		if hex == "" || !lonOK || !latOK {
			continue
		}
		positionTime, timeOK := rawFloat(state[3])
		if !timeOK || snapshot-int64(positionTime) > int64(positionMaxAge/time.Second) {
			continue
		}
		// Barometric altitude, falling back to geometric (state[13]) when the
		// aircraft reports no baro - otherwise an airborne aircraft with null
		// baro pinned to FL000 at terrain (finding #12).
		altitudeM, altOK := rawFloat(state[7])
		if !altOK {
			altitudeM, _ = rawFloat(state[13])
		}
		velocityMps, _ := rawFloat(state[9])
		track, _ := rawFloat(state[10])
		verticalMps, _ := rawFloat(state[11])
		lastContact, _ := rawFloat(state[4])
		contactAge := 0
		if lastContact > 0 {
			contactAge = int(snapshot - int64(lastContact))
			if contactAge < 0 {
				contactAge = 0
			}
		}
		category := 0
		if len(state) > 17 {
			if value, ok := rawFloat(state[17]); ok {
				category = int(value)
			}
		}
		payload, err := json.Marshal(map[string]any{
			"hex":                strings.ToLower(hex),
			"callsign":           rawString(state[1]),
			"originCountry":      rawString(state[2]),
			"kind":               kindForCategory(category),
			"squawk":             rawString(state[14]),
			"baroRateFpm":        verticalMps * 196.850394,
			"lat":                lat,
			"lon":                lon,
			"altFt":              altitudeM / 0.3048,
			"gsKt":               velocityMps * 1.943844,
			"track":              track,
			"onGround":           rawBool(state[8]),
			"validSeconds":       validSeconds,
			"lastContactAgeSec":  contactAge,
		})
		if err != nil {
			continue
		}
		records = append(records, plugins.RawRecord{
			SourcePluginID:   "opensky",
			SourceInstanceID: s.id,
			OriginalID:       fmt.Sprintf("opensky-%s-%s-%d", s.id, strings.ToLower(hex), now.Unix()),
			Domain:           "aviation",
			ObservedUTC:      now,
			Payload:          payload,
		})
	}
	return records, nil
}

func (s *Source) Start(ctx context.Context, output chan<- plugins.RawRecord) error {
	backoff := time.Duration(0)
	for {
		records, err := s.sample(ctx)
		if err != nil && ctx.Err() == nil {
			s.logger.Warn("opensky sample failed", "source", s.id, "error", err, "reason", s.StatusReason())
		}
		var limited errRateLimited
		if errors.As(err, &limited) {
			if backoff == 0 {
				backoff = limited.retryAfter
			} else {
				backoff = min(backoff*2, backoffMax)
			}
			backoff = max(backoff, limited.retryAfter)
		} else if err == nil {
			backoff = 0
			s.logger.Info("opensky snapshot", "source", s.id, "aircraft", len(records), "authMode", s.authMode)
		}
		for _, record := range records {
			select {
			case output <- record:
			case <-ctx.Done():
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.interval + backoff):
		}
	}
}
