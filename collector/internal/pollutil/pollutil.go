// Package pollutil holds small HTTP and disk-cache helpers shared by
// poll-style source plugins. It is intentionally generic: no provider
// names or domain vocabulary.
package pollutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const UserAgent = "ion-command-collector/0.1 (+https://github.com/svabi79/ion-command)"

type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e RateLimitedError) Error() string {
	return fmt.Sprintf("rate limited (retry after %s)", e.RetryAfter)
}

func RetryAfter(response *http.Response, fallback time.Duration) time.Duration {
	if response == nil {
		return fallback
	}
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
	return fallback
}

func Get(ctx context.Context, client *http.Client, rawURL string, headers map[string]string) ([]byte, error) {
	return Do(ctx, client, http.MethodGet, rawURL, "", nil, headers)
}

func Post(ctx context.Context, client *http.Client, rawURL, contentType string, body []byte, headers map[string]string) ([]byte, error) {
	return Do(ctx, client, http.MethodPost, rawURL, contentType, body, headers)
}

func Do(ctx context.Context, client *http.Client, method, rawURL, contentType string, body []byte, headers map[string]string) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 45 * time.Second}
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", UserAgent)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return nil, RateLimitedError{RetryAfter: RetryAfter(response, 5*time.Minute)}
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %s", rawURL, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 64<<20))
}

type FileCache struct {
	Path string
	TTL  time.Duration
}

func (c FileCache) Load() ([]byte, time.Time, bool) {
	if c.Path == "" {
		return nil, time.Time{}, false
	}
	info, err := os.Stat(c.Path)
	if err != nil {
		return nil, time.Time{}, false
	}
	if c.TTL > 0 && time.Since(info.ModTime()) > c.TTL {
		return nil, info.ModTime(), false
	}
	body, err := os.ReadFile(c.Path)
	if err != nil || len(body) == 0 {
		return nil, info.ModTime(), false
	}
	return body, info.ModTime(), true
}

func (c FileCache) Store(body []byte) error {
	if c.Path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	tmp := c.Path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.Path)
}

func ResolveCacheDir(configured, fallback string) string {
	if strings.TrimSpace(configured) != "" {
		return configured
	}
	return fallback
}
