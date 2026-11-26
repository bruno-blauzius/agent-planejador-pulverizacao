// Package copernicus downloads tiles of the Copernicus DEM GLO-30 elevation
// model from its public cloud-optimised GeoTIFF bucket (ADR 0021). The client
// only moves bytes: reading the GeoTIFF is the job of another adapter.
package copernicus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Defaults of Config.
const (
	DefaultBaseURL        = "https://copernicus-dem-30m.s3.amazonaws.com"
	DefaultRequestTimeout = 2 * time.Minute
	DefaultMaxBytes       = int64(120 << 20)
	DefaultRetries        = 2
	DefaultRetryBackoff   = 300 * time.Millisecond
	DefaultModelVersion   = "GLO-30 COG 2022-05"
	DefaultUserAgent      = "agentes-agro-api"

	modelSource      = "Copernicus DEM GLO-30"
	modelResolutionM = 30
)

// ErrTileTooLarge reports that the tile is larger than the configured read ceiling.
var ErrTileTooLarge = errors.New("tile larger than the read limit")

// tileKeyPattern is the format of a tile key: hemisphere and two-digit
// latitude of the south-west corner, hemisphere and three-digit longitude.
var tileKeyPattern = regexp.MustCompile(`^([NS][0-9]{2})([EW][0-9]{3})$`)

// Config holds the client settings; zero values take the defaults.
type Config struct {
	BaseURL        string
	RequestTimeout time.Duration // per attempt, including the body read
	MaxBytes       int64
	Retries        int // extra attempts after the first; negative means none
	RetryBackoff   time.Duration
	ModelVersion   string
	UserAgent      string
}

func (c Config) withDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = DefaultRequestTimeout
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.Retries == 0 {
		c.Retries = DefaultRetries
	}
	if c.Retries < 0 {
		c.Retries = 0
	}
	if c.RetryBackoff <= 0 {
		c.RetryBackoff = DefaultRetryBackoff
	}
	if c.ModelVersion == "" {
		c.ModelVersion = DefaultModelVersion
	}
	if c.UserAgent == "" {
		c.UserAgent = DefaultUserAgent
	}
	return c
}

// Client implements usecase.ElevationSource.
type Client struct {
	http *http.Client
	cfg  Config
}

// NewClient builds the client; a nil httpClient uses http.DefaultClient.
func NewClient(cfg Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{http: httpClient, cfg: cfg.withDefaults()}
}

// Model identifies the elevation model served by this client.
func (c *Client) Model() domain.ElevationModel {
	return domain.ElevationModel{Source: modelSource, ResolutionM: modelResolutionM, Version: c.cfg.ModelVersion}
}

// FetchTile downloads one 1x1 degree tile. A missing tile (the public bucket
// answers 403 or 404, e.g. over the ocean) and a failed download both wrap
// usecase.ErrElevationUnavailable; the caller closes the returned body.
func (c *Client) FetchTile(ctx context.Context, key domain.ElevationTileKey) (io.ReadCloser, error) {
	tileURL, err := c.tileURL(key)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, c.cfg.RetryBackoff<<(attempt-1)); err != nil {
				return nil, c.unavailable(key, err)
			}
		}
		body, retry, err := c.attempt(ctx, tileURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			break
		}
	}
	return nil, c.unavailable(key, lastErr)
}

func (c *Client) unavailable(key domain.ElevationTileKey, cause error) error {
	return fmt.Errorf("copernicus tile %s: %w: %w", key, usecase.ErrElevationUnavailable, cause)
}

// tileURL builds the object URL; the key is validated first so that no
// part of the path comes from unchecked input.
func (c *Client) tileURL(key domain.ElevationTileKey) (string, error) {
	parts := tileKeyPattern.FindStringSubmatch(string(key))
	if parts == nil {
		return "", fmt.Errorf("copernicus tile key %q: invalid format (want e.g. S22W047)", string(key))
	}
	name := fmt.Sprintf("Copernicus_DSM_COG_10_%s_00_%s_00_DEM", parts[1], parts[2])
	return c.cfg.BaseURL + "/" + name + "/" + name + ".tif", nil
}

// attempt performs one request; retry tells whether a failure is transient.
func (c *Client) attempt(ctx context.Context, tileURL string) (io.ReadCloser, bool, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodGet, tileURL, nil)
	if err != nil {
		cancel()
		return nil, false, fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("User-Agent", c.cfg.UserAgent)
	response, err := c.http.Do(request)
	if err != nil {
		cancel()
		// The URL is left out of the error: only the cause matters.
		return nil, ctx.Err() == nil, fmt.Errorf("request failed: %w", unwrapURLError(err))
	}
	transient, rejection := c.reject(response)
	if rejection != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		_ = response.Body.Close()
		cancel()
		return nil, transient, rejection
	}
	// The caller owns the body from here; closing it also releases the timeout.
	return &limitedBody{body: response.Body, remaining: c.cfg.MaxBytes, limit: c.cfg.MaxBytes, cancel: cancel}, false, nil
}

// reject explains why a response cannot be used, and whether trying again
// may help; the error is nil for a usable response.
func (c *Client) reject(response *http.Response) (bool, error) {
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusForbidden:
		return false, fmt.Errorf("tile does not exist (status %d)", response.StatusCode)
	default:
		transient := response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests
		return transient, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	if response.ContentLength > c.cfg.MaxBytes {
		return false, fmt.Errorf("%w: %d bytes, limit %d", ErrTileTooLarge, response.ContentLength, c.cfg.MaxBytes)
	}
	return false, nil
}

var _ usecase.ElevationSource = (*Client)(nil)

// unwrapURLError drops the *url.Error wrapper, which embeds the request URL.
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// limitedBody fails, instead of silently truncating, when the stream goes
// past the limit (the Content-Length may be absent or wrong).
type limitedBody struct {
	body      io.ReadCloser
	remaining int64
	limit     int64
	cancel    context.CancelFunc
}

func (l *limitedBody) Read(p []byte) (int, error) {
	if l.remaining <= 0 {
		var probe [1]byte
		n, err := l.body.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("%w: more than %d bytes", ErrTileTooLarge, l.limit)
		}
		return 0, err
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.body.Read(p)
	l.remaining -= int64(n)
	return n, err
}

func (l *limitedBody) Close() error {
	defer l.cancel()
	return l.body.Close()
}
