package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Common errors
var (
	ErrNotFound     = errors.New("resource not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrTimeout      = errors.New("request timeout")
	ErrServerError  = errors.New("server error")
)

// Client is the main MetaGuildNet SDK client
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	maxRetries int
	retryDelay time.Duration
}

// ClientOption configures a Client
type ClientOption func(*Client)

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(c *http.Client) ClientOption {
	return func(client *Client) {
		client.httpClient = c
	}
}

// WithTimeout sets the request timeout
func WithTimeout(d time.Duration) ClientOption {
	return func(client *Client) {
		client.httpClient.Timeout = d
	}
}

// WithMaxRetries sets the maximum number of retry attempts
func WithMaxRetries(n int) ClientOption {
	return func(client *Client) {
		client.maxRetries = n
	}
}

// WithRetryBackoff sets the retry backoff duration
func WithRetryBackoff(d time.Duration) ClientOption {
	return func(client *Client) {
		client.retryDelay = d
	}
}

// NewClient creates a new MetaGuildNet client
func NewClient(baseURL, token string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // For local dev; production should verify
				},
			},
		},
		maxRetries: 3,
		retryDelay: time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Clusters returns a cluster operations client
func (c *Client) Clusters() *ClusterClient {
	return &ClusterClient{client: c}
}

// Workspaces returns a workspace operations client for the given cluster
func (c *Client) Workspaces(clusterID string) *WorkspaceClient {
	return &WorkspaceClient{client: c, clusterID: clusterID}
}

// Databases returns a database operations client for the given cluster
func (c *Client) Databases(clusterID string) *DatabaseClient {
	return &DatabaseClient{client: c, clusterID: clusterID}
}

// Health returns a health operations client
func (c *Client) Health() *HealthClient {
	return &HealthClient{client: c}
}

// doRequest executes an HTTP request with retries
func (c *Client) doRequest(ctx context.Context, method, path string, body any, result any) error {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryDelay * time.Duration(attempt)):
			}
		}

		err := c.doRequestOnce(ctx, method, path, body, result)
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry on client errors (4xx except 429)
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnauthorized) {
			return err
		}
	}

	return lastErr
}

func (c *Client) doRequestOnce(ctx context.Context, method, path string, body any, result any) error {
	url := c.baseURL + path

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ErrTimeout
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle status codes
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// Success
		if result != nil {
			if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
				return fmt.Errorf("failed to decode response: %w", err)
			}
		}
		return nil

	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound

	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrUnauthorized

	case resp.StatusCode >= 500:
		return ErrServerError

	default:
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}
}

// get is a convenience method for GET requests
func (c *Client) get(ctx context.Context, path string, result any) error {
	return c.doRequest(ctx, http.MethodGet, path, nil, result)
}

// post is a convenience method for POST requests
func (c *Client) post(ctx context.Context, path string, body any, result any) error {
	return c.doRequest(ctx, http.MethodPost, path, body, result)
}

// put is a convenience method for PUT requests
func (c *Client) put(ctx context.Context, path string, body any, result any) error {
	return c.doRequest(ctx, http.MethodPut, path, body, result)
}

// delete is a convenience method for DELETE requests
func (c *Client) delete(ctx context.Context, path string) error {
	return c.doRequest(ctx, http.MethodDelete, path, nil, nil)
}
