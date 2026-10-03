package nanogpt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrNoAPIKey = errors.New("NANOGPT_API_KEY is required")

const (
	DefaultBaseURL = "https://nano-gpt.com"

	// RequestTimeout stays under internal/server's write timeout, so a slow call returns an
	// error to the caller instead of a response the server cuts off mid-flight.
	RequestTimeout = 90 * time.Second

	errorBodyLimit = 512
)

type Config struct {
	APIKey  string
	BaseURL string
}

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, ErrNoAPIKey
	}

	baseURL := strings.TrimSuffix(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		http:    &http.Client{Timeout: RequestTimeout},
	}, nil
}

// credential is the header an endpoint reads the key from; the data endpoints disagree on
// which header that is, so each caller names its own.
type credential struct {
	header string
	value  string
}

func (c *Client) post(ctx context.Context, path string, cred credential, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", path, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", path, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(cred.header, cred.value)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", path, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("call %s: %s: %s", path, resp.Status, errorSnippet(resp.Body))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}

	return nil
}

func errorSnippet(body io.Reader) string {
	snippet, _ := io.ReadAll(io.LimitReader(body, errorBodyLimit))

	return strings.TrimSpace(string(snippet))
}
