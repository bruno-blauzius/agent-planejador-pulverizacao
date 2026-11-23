package keycloak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	// maxResponseBytes caps what is read from any response, so a misbehaving
	// server cannot exhaust memory.
	maxResponseBytes = 1 << 20
	// tokenExpiryMargin renews the token a little before it expires.
	tokenExpiryMargin = 20 * time.Second
)

// ErrEmailNotSent means the provider accepted the request but could not send
// the action e-mail (SMTP not configured or failing). Use cases turn it into
// EmailNotSent without invalidating the invitation.
var ErrEmailNotSent = errors.New("action e-mail not sent")

// ErrAccountExists is the cause of a 409 on account creation (a concurrent
// request created the same e-mail). It is wrapped in ErrIdentityProviderUnavailable.
var ErrAccountExists = errors.New("account already exists in the realm")

// statusError is a non-2xx answer. It deliberately carries no response body:
// bodies may echo user data (e-mail, names).
type statusError struct {
	operation string
	status    int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("keycloak %s: unexpected status %d", e.operation, e.status)
}

// client is the low-level Admin API caller: token cache, timeouts, limited
// retry of GETs and error sanitizing.
type client struct {
	cfg   Config
	http  *http.Client
	log   *slog.Logger
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time

	mu          sync.Mutex // guards token and tokenExpiry
	token       string
	tokenExpiry time.Time
}

func newClient(cfg Config) *client {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			// Never follow redirects: the bearer token must not travel to another host.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &client{cfg: cfg, http: httpClient, log: logger, sleep: sleepContext, now: time.Now}
}

// closeBody closes a response body that was already read. A failure here
// cannot change the outcome of the exchange, so it is only logged.
func (c *client) closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		c.log.Debug("keycloak response body close failed", slog.String("error", err.Error()))
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *client) adminURL(parts ...string) string {
	var b strings.Builder
	b.WriteString(c.cfg.BaseURL)
	b.WriteString("/admin/realms/")
	b.WriteString(url.PathEscape(c.cfg.Realm))
	for _, part := range parts {
		b.WriteByte('/')
		b.WriteString(url.PathEscape(part))
	}
	return b.String()
}

// accessToken returns a cached token or fetches a new one (client credentials).
func (c *client) accessToken(ctx context.Context, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && c.token != "" && c.now().Before(c.tokenExpiry) {
		return c.token, nil
	}
	c.token = ""

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}
	endpoint := c.cfg.BaseURL + "/realms/" + url.PathEscape(c.cfg.Realm) + "/protocol/openid-connect/token"
	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("keycloak token: %w", transportCause(err))
	}
	defer c.closeBody(resp.Body)
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("keycloak token: read response: %w", transportCause(err))
	}
	if resp.StatusCode != http.StatusOK {
		return "", &statusError{operation: "token", status: resp.StatusCode}
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.AccessToken == "" {
		return "", errors.New("keycloak token: malformed response")
	}
	lifetime := max(time.Duration(payload.ExpiresIn)*time.Second-tokenExpiryMargin, 0)
	c.token = payload.AccessToken
	c.tokenExpiry = c.now().Add(lifetime)
	return c.token, nil
}

// transportCause drops the request URL that *url.Error embeds (it can hold an
// e-mail in the query string) and keeps the cause matchable with errors.Is.
func transportCause(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

type request struct {
	operation string
	method    string
	url       string
	query     url.Values
	body      any
	// out receives the decoded JSON body of a 2xx answer when not nil.
	out any
}

type response struct {
	status int
	header http.Header
}

// do sends the request. A non-2xx answer yields *statusError (callers inspect
// the status for 404/409). GETs are retried on network errors and 5xx; a 401
// forces one token refresh for any method (a 401 means nothing was executed).
func (c *client) do(ctx context.Context, r request) (response, error) {
	attempts := 1
	if r.method == http.MethodGet {
		attempts += c.cfg.MaxGETRetries
	}
	refreshed := false
	var lastErr error
	for attempt := range attempts {
		if attempt > 0 {
			if err := c.sleep(ctx, time.Duration(attempt)*c.cfg.RetryBackoff); err != nil {
				return response{}, fmt.Errorf("keycloak %s: %w", r.operation, err)
			}
		}
		resp, err := c.once(ctx, r, false)
		if statusOf(err) == http.StatusUnauthorized && !refreshed {
			refreshed = true
			resp, err = c.once(ctx, r, true)
		}
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable(err) {
			break
		}
	}
	return response{}, lastErr
}

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var statusErr *statusError
	if errors.As(err, &statusErr) {
		return statusErr.status >= 500
	}
	// Network errors and timeouts of one attempt; malformed responses are not
	// transient but retrying a GET is harmless.
	return true
}

func (c *client) once(ctx context.Context, r request, forceToken bool) (response, error) {
	token, err := c.accessToken(ctx, forceToken)
	if err != nil {
		return response{}, fmt.Errorf("%s: authenticate: %w", r.operation, err)
	}
	var body io.Reader
	if r.body != nil {
		encoded, err := json.Marshal(r.body)
		if err != nil {
			return response{}, fmt.Errorf("%s: encode body: %w", r.operation, err)
		}
		body = bytes.NewReader(encoded)
	}
	endpoint := r.url
	if len(r.query) > 0 {
		endpoint += "?" + r.query.Encode()
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, r.method, endpoint, body)
	if err != nil {
		return response{}, fmt.Errorf("%s: build request: %w", r.operation, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	started := c.now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Debug("keycloak request failed", slog.String("operation", r.operation), slog.Duration("elapsed", c.now().Sub(started)))
		return response{}, fmt.Errorf("keycloak %s: %w", r.operation, transportCause(err))
	}
	defer c.closeBody(resp.Body)
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	c.log.Debug("keycloak request",
		slog.String("operation", r.operation),
		slog.Int("status", resp.StatusCode),
		slog.Duration("elapsed", c.now().Sub(started)))
	if readErr != nil {
		return response{}, fmt.Errorf("keycloak %s: read response: %w", r.operation, transportCause(readErr))
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return response{}, &statusError{operation: r.operation, status: resp.StatusCode}
	}
	if r.out != nil && len(raw) > 0 {
		if len(raw) > maxResponseBytes {
			return response{}, fmt.Errorf("keycloak %s: response too large", r.operation)
		}
		if err := json.Unmarshal(raw, r.out); err != nil {
			return response{}, fmt.Errorf("keycloak %s: malformed response", r.operation)
		}
	}
	return response{status: resp.StatusCode, header: resp.Header}, nil
}

// statusOf returns the HTTP status carried by err, or 0.
func statusOf(err error) int {
	var statusErr *statusError
	if errors.As(err, &statusErr) {
		return statusErr.status
	}
	return 0
}

// unavailable maps any provider failure to usecase.ErrIdentityProviderUnavailable,
// keeping the cause (errors.Is / errors.As still work on it).
func unavailable(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s: %w", usecase.ErrIdentityProviderUnavailable, operation, err)
}
