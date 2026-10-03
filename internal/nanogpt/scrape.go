package nanogpt

import (
	"context"
	"fmt"
)

const (
	scrapePath = "/api/scrape-urls"

	// ProviderAuto lets NanoGPT route between the crawler and Linkup.
	ProviderAuto = "auto"

	MaxScrapeURLs = 5
)

type ScrapeRequest struct {
	URLs        []string `json:"urls"`
	StealthMode bool     `json:"stealthMode,omitempty"`
	Provider    string   `json:"provider,omitempty"`
}

type ScrapeResponse struct {
	Results []ScrapeResult `json:"results"`
	// Summary holds per-request stats such as stealthModeUsed. NanoGPT documents only part
	// of it, so it stays raw rather than dropping fields the API chose to send.
	Summary any `json:"summary"`
}

type ScrapeResult struct {
	URL      string `json:"url"`
	Success  bool   `json:"success"`
	Title    string `json:"title,omitempty"`
	Content  string `json:"content,omitempty"`
	Markdown string `json:"markdown,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (c *Client) Scrape(ctx context.Context, req ScrapeRequest) (*ScrapeResponse, error) {
	if len(req.URLs) == 0 || len(req.URLs) > MaxScrapeURLs {
		return nil, fmt.Errorf("scrape takes 1 to %d URLs, got %d", MaxScrapeURLs, len(req.URLs))
	}

	if req.Provider == "" {
		req.Provider = ProviderAuto
	}

	var out ScrapeResponse

	cred := credential{header: "Authorization", value: "Bearer " + c.apiKey}
	if err := c.post(ctx, scrapePath, cred, req, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
