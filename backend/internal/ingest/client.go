package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is the shared polite HTTP client: identifying User-Agent, overall
// timeouts, per-host spacing, retries on transient failures only.
type Client struct {
	http      *http.Client
	userAgent string
	minGap    time.Duration
	retries   int
	backoff   time.Duration
	robots    *RobotsCache

	mu   sync.Mutex
	last map[string]time.Time
}

type ClientOptions struct {
	UserAgent string
	Timeout   time.Duration
	MinGap    time.Duration
	Retries   int
	Backoff   time.Duration
}

// NewClient builds a Client.
func NewClient(opts ClientOptions) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.MinGap <= 0 {
		opts.MinGap = time.Second
	}
	if opts.Retries < 0 {
		opts.Retries = 0
	}
	if opts.Backoff <= 0 {
		opts.Backoff = 2 * time.Second
	}
	c := &Client{
		http:      &http.Client{Timeout: opts.Timeout},
		userAgent: opts.UserAgent,
		minGap:    opts.MinGap,
		retries:   opts.Retries,
		backoff:   opts.Backoff,
		robots:    NewRobotsCache(),
		last:      map[string]time.Time{},
	}
	c.robots.SetClient(c)
	return c
}

// Response is a fully-read successful HTTP response.
type Response struct {
	Body   []byte
	Header http.Header
	Status int
}

// Get performs a GET with retry, rate limiting and the shared user agent.
func (c *Client) Get(ctx context.Context, rawURL string) (*Response, error) {
	return c.GetWithHeaders(ctx, rawURL, nil)
}

func (c *Client) GetWithHeaders(ctx context.Context, rawURL string, headers map[string]string) (*Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	host := u.Hostname()

	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, c.backoff*time.Duration(1<<(attempt-1))); err != nil {
				return nil, err
			}
		}
		if err := c.wait(ctx, host); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.userAgent)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if retryableErr(err) && attempt < c.retries {
				lastErr = err
				continue
			}
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		if readErr != nil {
			if retryableErr(readErr) && attempt < c.retries {
				lastErr = readErr
				continue
			}
			return nil, readErr
		}
		if retryableStatus(resp.StatusCode) && attempt < c.retries {
			lastErr = &StatusError{URL: rawURL, Status: resp.StatusCode}
			if wait := retryAfter(resp.Header); wait > 0 {
				if err := sleep(ctx, wait); err != nil {
					return nil, err
				}
			}
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &StatusError{URL: rawURL, Status: resp.StatusCode, Body: string(body)}
		}
		return &Response{Body: body, Header: resp.Header, Status: resp.StatusCode}, nil
	}
	return nil, lastErr
}

// StatusError is a non-2xx response surfaced to a source.
type StatusError struct {
	URL    string
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("GET %s: status %d", e.URL, e.Status)
}

// IsTransient reports whether an error is worth retrying.
func IsTransient(err error) bool {
	return retryableErr(err) || retryableStatus(statusOf(err))
}

func statusOf(err error) int {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status
	}
	return 0
}

func retryableStatus(code int) bool {
	if code == http.StatusTooManyRequests {
		return true
	}
	return code >= 500 && code <= 599
}

// retryableErr is true only for timeouts and connection errors.
func retryableErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "eof")
}

func retryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// wait enforces the per-host minimum gap.
func (c *Client) wait(ctx context.Context, host string) error {
	c.mu.Lock()
	last := c.last[host]
	c.mu.Unlock()
	if !last.IsZero() {
		if gap := time.Until(last.Add(c.minGap)); gap > 0 {
			if err := sleep(ctx, gap); err != nil {
				return err
			}
		}
	}
	c.mu.Lock()
	c.last[host] = time.Now()
	c.mu.Unlock()
	return nil
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
