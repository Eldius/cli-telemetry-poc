// Package curseforge provides functionality for interacting with the CurseForge API.
package curseforge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eldius/initial-config-go/logs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// DefaultBaseURL is the CurseForge API base URL.
const DefaultBaseURL = "https://api.curseforge.com"

const (
	userAgent        = "minecraft-mods-db/0.1.0 (+https://github.com/eldius/minecraft-mods-db)"
	maxRetries       = 6
	initialBackoff   = 1 * time.Second
	maxBackoff       = 30 * time.Second
	maxErrorBodySize = 4 << 10
	meterName        = "github.com/eldius/cli-telemetry-poc/client/curseforge"
)

// ErrAPIKeyRequired is returned when no API key is configured.
var ErrAPIKeyRequired = errors.New("curseforge API key is required (set CURSEFORGE_API_KEY; get one at https://console.curseforge.com)")

// APIError is a non-successful response from the CurseForge API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("curseforge API error: status %d: %s", e.StatusCode, e.Message)
}

// Config configures a Client.
type Config struct {
	BaseURL           string
	APIKey            string
	GameID            int
	RequestsPerSecond float64
	HTTPClient        *http.Client
}

// Client is a CurseForge API curseforge with rate limiting and retries.
type Client struct {
	baseURL  string
	apiKey   string
	gameID   int
	http     *http.Client
	limiter  *rateLimiter
	counters *clientCounters
}

type clientCounters struct {
	modsFetched metric.Int64Histogram
	apiCalls    metric.Int64Counter
}

// New creates a Client.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, ErrAPIKeyRequired
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.GameID == 0 {
		cfg.GameID = GameIDMinecraft
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}

	var counters clientCounters

	if mp := otel.GetMeterProvider(); mp != nil {
		meter := mp.Meter(
			meterName,
			metric.WithSchemaURL(meterName),
		)
		var err error
		counters.modsFetched, err = meter.Int64Histogram("curseforge.mods_search.fetched")
		if err != nil {
			return nil, fmt.Errorf("meter curseforge.mods_search.fetched failed: %w", err)
		}
		counters.apiCalls, err = meter.Int64Counter("curseforge.mods_search.api_call")
		if err != nil {
			return nil, fmt.Errorf("meter curseforge.mods_search.api_call failed: %w", err)
		}
	}
	return &Client{
		baseURL:  strings.TrimSuffix(cfg.BaseURL, "/"),
		apiKey:   cfg.APIKey,
		gameID:   cfg.GameID,
		http:     cfg.HTTPClient,
		limiter:  newRateLimiter(cfg.RequestsPerSecond),
		counters: &counters,
	}, nil
}

// Close releases curseforge resources (stops the rate limiter).
func (c *Client) Close() {
	c.limiter.stop()
}

// SearchMods searches for mods matching the given filters.
func (c *Client) SearchMods(ctx context.Context, p SearchParams) (*SearchResult, error) {
	if p.PageSize <= 0 {
		p.PageSize = 50
	}
	if p.SortField == 0 {
		p.SortField = SortFieldPopularity
	}
	if p.SortOrder == "" {
		p.SortOrder = SortOrderDesc
	}

	q := url.Values{
		"gameId":    {strconv.Itoa(c.gameID)},
		"pageSize":  {strconv.Itoa(p.PageSize)},
		"index":     {strconv.Itoa(p.Index)},
		"sortField": {strconv.Itoa(p.SortField)},
		"sortOrder": {p.SortOrder},
	}
	if p.ClassID != 0 {
		q.Set("classId", strconv.Itoa(p.ClassID))
	}
	if p.CategoryID != 0 {
		q.Set("categoryId", strconv.Itoa(p.CategoryID))
	}
	if p.GameVersion != "" {
		q.Set("gameVersion", p.GameVersion)
	}
	if p.ModLoaderType != 0 {
		q.Set("modLoaderType", strconv.Itoa(p.ModLoaderType))
	}

	raw, err := c.get(ctx, "/v1/mods/search", q)
	if err != nil {
		return nil, fmt.Errorf("searching mods: %w", err)
	}

	var page struct {
		Data       []json.RawMessage `json:"data"`
		Pagination Pagination        `json:"pagination"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("decoding search response: %w", err)
	}

	res := &SearchResult{Pagination: page.Pagination}
	for _, r := range page.Data {
		var m Mod
		if err := json.Unmarshal(r, &m); err != nil {
			return nil, fmt.Errorf("decoding mod: %w", err)
		}
		c.registerModsFetched(ctx, int64(len(res.Mods)))
		res.Mods = append(res.Mods, ModEnvelope{Mod: m, Raw: r})
	}
	return res, nil
}

// GetCategories lists categories for the game. Pass classID 0 to list all
// categories (including the class entries themselves).
func (c *Client) GetCategories(ctx context.Context, classID int) ([]Category, error) {
	q := url.Values{"gameId": {strconv.Itoa(c.gameID)}}
	if classID != 0 {
		q.Set("classId", strconv.Itoa(classID))
	}
	raw, err := c.get(ctx, "/v1/categories", q)
	if err != nil {
		return nil, fmt.Errorf("getting categories: %w", err)
	}
	var res struct {
		Data []Category `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decoding categories response: %w", err)
	}
	return res.Data, nil
}

// GetGameVersions lists the game versions known to CurseForge for the game.
func (c *Client) GetGameVersions(ctx context.Context) ([]GameVersion, error) {
	raw, err := c.get(ctx, fmt.Sprintf("/v1/games/%d/versions", c.gameID), nil)
	if err != nil {
		return nil, fmt.Errorf("getting game versions: %w", err)
	}
	var res struct {
		Data []GameVersion `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("decoding game versions response: %w", err)
	}
	return res.Data, nil
}

// get performs a rate-limited GET with retries on transient failures and
// returns the raw response body.
func (c *Client) get(ctx context.Context, path string, q url.Values) (json.RawMessage, error) {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("User-Agent", userAgent)

	var lastErr error
	backoff := initialBackoff
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := c.limiter.wait(ctx); err != nil {
			return nil, err
		}

		resp, err := c.http.Do(req) //nolint:bodyclose // It will be closed at `readAndClose`
		c.registerApiCall(ctx)
		if err != nil {
			lastErr = err
		} else {
			body, readErr := readAndClose(resp)
			if readErr != nil {
				lastErr = readErr
			} else if resp.StatusCode == http.StatusOK {
				return body, nil
			} else {
				lastErr = &APIError{StatusCode: resp.StatusCode, Message: truncate(string(body), maxErrorBodySize)}
			}

			if !isRetryable(resp.StatusCode) {
				return nil, lastErr
			}
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
					backoff = time.Duration(secs) * time.Second
				}
			}
		}

		logs.NewLogger(ctx, logs.KeyValueData{
			"attempt": attempt,
			"url":     u,
			"backoff": backoff.String(),
		}).WithError(lastErr).Warn("curseforge request failed, retrying")

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	return nil, fmt.Errorf("request failed after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) registerApiCall(ctx context.Context) {
	if c.counters.apiCalls == nil {
		return
	}
	c.counters.apiCalls.Add(ctx, 1)
}

func (c *Client) registerModsFetched(ctx context.Context, count int64) {
	if c.counters.modsFetched == nil {
		return
	}
	c.counters.modsFetched.Record(ctx, count)
}

func isRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

func readAndClose(resp *http.Response) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// rateLimiter spaces out requests using a ticker.
type rateLimiter struct {
	ticker *time.Ticker
}

func newRateLimiter(rps float64) *rateLimiter {
	if rps <= 0 {
		rps = 5
	}
	interval := time.Duration(float64(time.Second) / rps)
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	return &rateLimiter{ticker: time.NewTicker(interval)}
}

func (r *rateLimiter) wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ticker.C:
		return nil
	}
}

func (r *rateLimiter) stop() {
	r.ticker.Stop()
}
