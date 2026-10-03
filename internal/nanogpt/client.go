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

	// FastTimeout bounds the endpoints that answer in one round trip. A call that asks
	// nano-gpt to wait for a crawl passes its own budget instead.
	FastTimeout = 90 * time.Second

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
		http:    &http.Client{},
	}, nil
}

// credential is the header an endpoint reads the key from; the data endpoints disagree on
// which header that is, so each caller names its own.
type credential struct {
	header string
	value  string
}

func (c *Client) post(ctx context.Context, path string, budget time.Duration, cred credential, payload, out any) error {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

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
