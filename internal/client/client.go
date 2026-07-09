// Package client is a thin HTTP client for the Claude Managed Agents API
// (agents, environments, and scheduled deployments). It handles auth, the
// required beta header, JSON encoding, and typed error responses.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the Claude API root.
	DefaultBaseURL = "https://api.anthropic.com"
	// DefaultAnthropicVersion is the stable API version header value.
	DefaultAnthropicVersion = "2023-06-01"
	// BetaHeader gates the Managed Agents endpoints. Required on every request.
	BetaHeader = "managed-agents-2026-04-01"
)

// Client talks to the Claude Managed Agents API.
type Client struct {
	httpClient       *http.Client
	baseURL          string
	apiKey           string
	anthropicVersion string
	userAgent        string
}

// Option customizes a Client.
type Option func(*Client)

// WithBaseURL overrides the API root (e.g. for a gateway or test server).
func WithBaseURL(u string) Option {
	return func(c *Client) {
		if u != "" {
			c.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithAnthropicVersion overrides the anthropic-version header.
func WithAnthropicVersion(v string) Option {
	return func(c *Client) {
		if v != "" {
			c.anthropicVersion = v
		}
	}
}

// WithHTTPClient injects a custom *http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithUserAgent sets the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// New builds a Client. apiKey is required.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		httpClient:       &http.Client{Timeout: 60 * time.Second},
		baseURL:          DefaultBaseURL,
		apiKey:           apiKey,
		anthropicVersion: DefaultAnthropicVersion,
		userAgent:        "terraform-provider-claude-managed-agents",
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// APIError is a non-2xx response from the API.
type APIError struct {
	StatusCode int
	Type       string
	Message    string
	RequestID  string
	RawBody    string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.RawBody
	}
	if e.Type != "" {
		return fmt.Sprintf("claude api error (%d %s): %s", e.StatusCode, e.Type, msg)
	}
	return fmt.Sprintf("claude api error (%d): %s", e.StatusCode, msg)
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if e, ok := err.(*APIError); ok {
		apiErr = e
	}
	return apiErr != nil && apiErr.StatusCode == http.StatusNotFound
}

// IsConflict reports whether err is a 409 from the API (version mismatch,
// or a resource that cannot be deleted because it is still referenced).
func IsConflict(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.StatusCode == http.StatusConflict
	}
	return false
}

// errorEnvelope matches the standard Anthropic error body.
type errorEnvelope struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// do issues a request. If body is non-nil it is JSON-encoded. If out is
// non-nil a 2xx response body is decoded into it. A 204 leaves out untouched.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", c.anthropicVersion)
	req.Header.Set("anthropic-beta", BetaHeader)
	req.Header.Set("user-agent", c.userAgent)
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			RequestID:  resp.Header.Get("request-id"),
			RawBody:    string(respBody),
		}
		var env errorEnvelope
		if json.Unmarshal(respBody, &env) == nil {
			apiErr.Type = env.Error.Type
			apiErr.Message = env.Error.Message
		}
		return apiErr
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decoding response body: %w", err)
		}
	}
	return nil
}
