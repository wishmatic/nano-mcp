// Package resolve turns an address an agent names into the bytes of an image. A provider that
// takes an image cannot read a file this deployment holds, or a host only this deployment can
// reach, so the bytes travel with the request instead of an address.
package resolve

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/wishmatic/nano-mcp/internal/sourcemap"
	"github.com/wishmatic/nano-mcp/internal/utils"
)

const (
	fetchTimeout = 60 * time.Second
	maxRedirects = 10

	// Wikimedia and similar sites answer generic or missing agents with a 403.

	userAgent = "nano-mcp/0.1.0 (https://github.com/wishmatic/nano-mcp; bot)"
)

type Client struct {
	sources *sourcemap.Map
	http    *http.Client

	// maxBytes is MaxImageBytes, and is a field so a test can drive the limit without
	// building a sixteen megabyte image.
	maxBytes int64
}

// New takes the address map, which may be nil when nothing needs translating.
func New(sources *sourcemap.Map) *Client {
	return &Client{
		sources:  sources,
		maxBytes: MaxImageBytes,
		http: &http.Client{
			Timeout: fetchTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Fetch returns the bytes for rawURL, following redirects itself rather than through the HTTP
// client, so a mapped host is rewritten on every hop instead of only the first.
func (c *Client) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("resolve: parse URL %q: %w", rawURL, err)
	}

	for hop := 0; ; hop++ {
		if source, rest, ok := c.sources.Lookup(u); ok {
			switch source.Kind() {
			case sourcemap.Directory:
				return source.Read(rest)
			case sourcemap.BaseURL:
				rewritten, err := source.Target(u, rest)
				if err != nil {
					return nil, err
				}

				u = rewritten
			}
		}

		resp, err := c.get(ctx, u)
		if err != nil {
			return nil, err
		}

		if !isRedirect(resp) {
			return readResponse(u, resp)
		}

		location := resp.Header.Get("Location")
		resp.Body.Close()

		if location == "" {
			return nil, fmt.Errorf("resolve: %s returned HTTP %d without a Location header", u, resp.StatusCode)
		}

		if hop >= maxRedirects {
			return nil, fmt.Errorf("resolve: too many redirects fetching %q", rawURL)
		}

		next, err := u.Parse(location)
		if err != nil {
			return nil, fmt.Errorf("resolve: invalid redirect location %q: %w", location, err)
		}

		u = next
	}
}

func (c *Client) get(ctx context.Context, u *url.URL) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("resolve: build request for %s: %w", u, err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("resolve: fetch %s: %w", u, err)
	}

	return resp, nil
}

func isRedirect(resp *http.Response) bool {
	return resp.StatusCode >= 300 && resp.StatusCode < 400
}

func readResponse(u *url.URL, resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resolve: %s returned HTTP %d: %s", u, resp.StatusCode, utils.ReadLimited(resp.Body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("resolve: read %s: %w", u, err)
	}

	return data, nil
}
