package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Status is the integration health state Veylix records (REQUIREMENTS §7).
// It is derived from an actual HTTP outcome — never assumed.
type Status string

const (
	StatusActive                Status = "ACTIVE"
	StatusReauthRequired        Status = "REAUTH_REQUIRED"
	StatusConnectionUnavailable Status = "CONNECTION_UNAVAILABLE"
	StatusPendingVerification   Status = "PENDING_VERIFICATION"
)

// Client talks to exactly one 3X-UI panel with a Bearer token.
//
// Security properties (new_update.md §5, §7):
//   - the token is held in memory only and never logged or serialised;
//   - one reused transport with bounded pools (no connection storm on a 1 vCPU box);
//   - every request has a context deadline;
//   - retries are performed ONLY for idempotent GETs.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// Options configures a Client.
type Options struct {
	// Target is the validated panel endpoint.
	Target *Target
	// Token is the panel API token (full-admin credential).
	Token string
	// Timeout applies to a single request. Defaults to 10s.
	Timeout time.Duration
	// Transport may be injected by tests; nil uses the hardened default.
	Transport http.RoundTripper
}

const (
	defaultTimeout   = 10 * time.Second
	maxIdleConns     = 8
	idleConnIdle     = 90 * time.Second
	maxResponseBytes = 8 << 20 // 8 MiB: guards against a hostile/oversized body
	maxGetRetries    = 2
	retryBackoff     = 300 * time.Millisecond
)

// New builds a Client. It refuses an empty token or target.
func New(opts Options) (*Client, error) {
	if opts.Target == nil || opts.Target.BaseURL == "" {
		return nil, errors.New("panel: target is required")
	}
	if opts.Token == "" {
		return nil, errors.New("panel: token is required")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	transport := opts.Transport
	if transport == nil {
		transport = &http.Transport{
			// TLS verification is never disabled (Bible §8).
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConns,
			IdleConnTimeout:       idleConnIdle,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: time.Second,
			// A redirect to another host would bypass the SSRF checks made at
			// registration time, so redirects are refused outright.
			DisableKeepAlives: false,
		}
	}

	return &Client{
		base:  opts.Target.BaseURL,
		token: opts.Token,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse // never follow redirects
			},
		},
	}, nil
}

// StatusError is returned when the panel answered with an unexpected outcome.
// It never contains the token or the raw body.
type StatusError struct {
	HTTPStatus int
	Endpoint   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("panel: %s returned HTTP %d", e.Endpoint, e.HTTPStatus)
}

// do performs one authenticated request and decodes the envelope object.
//
// gotResponse reports whether the panel actually answered (so callers can tell
// "the remote rejected us" from "we never reached it").
func (c *Client) do(ctx context.Context, method, path string, body any) (json.RawMessage, int, error) {
	// The encoded payload is kept as BYTES, not as a reader.
	//
	// Before this, one *bytes.Reader was created outside the loop and reused on
	// every attempt. A reader is consumed by the first request, so retry #2 sent
	// an EMPTY body while still claiming Content-Type: application/json — the
	// panel would answer 200 and silently do nothing. It was latent only because
	// every retried call today is a GET with no body; the first GET with a body,
	// or a decision to retry a write, would have shipped the bug.
	var raw []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("panel: encode body: %w", err)
		}
		raw = encoded
	}

	attempts := 1
	if method == http.MethodGet {
		attempts = 1 + maxGetRetries // GET is idempotent; writes never retry blindly
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(retryBackoff * time.Duration(attempt)):
			}
		}

		// A fresh reader per attempt: never re-send an exhausted stream.
		var payload io.Reader
		if raw != nil {
			payload = bytes.NewReader(raw)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, payload)
		if err != nil {
			return nil, 0, fmt.Errorf("panel: build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
		if raw != nil {
			req.Header.Set("Content-Type", "application/json")
			req.ContentLength = int64(len(raw))
		}

		resp, err := c.http.Do(req)
		if err != nil {
			// Transport-level failure (timeout, DNS, TLS). Retry for GETs only.
			lastErr = err
			continue
		}

		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		return bodyBytes, resp.StatusCode, nil
	}

	return nil, 0, fmt.Errorf("panel: request failed: %w", lastErr)
}

// decodeEnvelope unwraps {success, msg, obj}.
func decodeEnvelope(raw []byte) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, errors.New("panel: empty response body")
	}
	var env envelope[json.RawMessage]
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("panel: malformed response: %w", err)
	}
	if !env.Success {
		msg := env.Msg
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("panel: request rejected: %s", msg)
	}
	return env.Obj, nil
}

// Classify maps an HTTP outcome to the integration status (D-008 mapping).
//
// 404 is deliberately NOT "invalid token": a wrong base path also returns 404
// (observed on the verified panel), so it is treated as an unavailable
// connection and reported with a hint.
func Classify(httpStatus int, transportErr error) Status {
	if transportErr != nil {
		return StatusConnectionUnavailable
	}
	switch {
	case httpStatus == http.StatusOK:
		return StatusActive
	case httpStatus == http.StatusUnauthorized || httpStatus == http.StatusForbidden:
		return StatusReauthRequired
	default:
		return StatusConnectionUnavailable
	}
}
