package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

type webScrapeInput struct {
	URLs        []string `json:"urls" jsonschema:"1 to 5 public http(s) URLs to fetch"`
	Provider    string   `json:"provider,omitempty" jsonschema:"auto routes between nano-gpt's crawler and Linkup, linkup fetches without a fallback"`
	StealthMode bool     `json:"stealthMode,omitempty" jsonschema:"route through a stealth proxy for anti-bot defences, at 5x the per-URL charge"`
}

type webScrapeOutput struct {
	Results []scrapeResult `json:"results" jsonschema:"one entry per requested URL, in the order they were asked for"`
	Summary any            `json:"summary" jsonschema:"nano-gpt's request stats, including stealthModeUsed"`
}

type scrapeResult struct {
	URL      string `json:"url" jsonschema:"URL that was scraped"`
	Success  bool   `json:"success" jsonschema:"whether the page was fetched"`
	Title    string `json:"title,omitempty" jsonschema:"page title when the provider reported one"`
	Content  string `json:"content,omitempty" jsonschema:"page text"`
	Markdown string `json:"markdown,omitempty" jsonschema:"markdown rendering of the page"`
	Error    string `json:"error,omitempty" jsonschema:"why the fetch failed, when it did"`
}

func registerWebScrape(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "web_scrape",
		Description: "Fetch the readable text of up to 5 public web pages through nano-gpt. Returns formatted " +
			"text, never raw HTML. Costs about 0.0015 per URL, or 0.0075 with stealthMode.",
	}, h.webScrape)
}

func (h *handlers) webScrape(ctx context.Context, _ *mcp.CallToolRequest, in webScrapeInput) (
	*mcp.CallToolResult, webScrapeOutput, error,
) {
	req := nanogpt.ScrapeRequest{
		URLs:        in.URLs,
		Provider:    in.Provider,
		StealthMode: in.StealthMode,
	}

	resp, err := h.nanogpt.Scrape(ctx, req)
	if err != nil {
		return nil, webScrapeOutput{}, h.fail("web_scrape", err)
	}

	results := make([]scrapeResult, 0, len(resp.Results))
	for _, result := range resp.Results {
		results = append(results, scrapeResult{
			URL:      result.URL,
			Success:  result.Success,
			Title:    result.Title,
			Content:  result.Content,
			Markdown: result.Markdown,
			Error:    result.Error,
		})
	}

	return nil, webScrapeOutput{Results: results, Summary: resp.Summary}, nil
}
