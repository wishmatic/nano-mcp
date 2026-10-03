package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

// firecrawlOutput passes the upstream payload through unmodelled: nano-gpt documents no
// response shape for this endpoint, and the three operations do not share one.
type firecrawlOutput struct {
	Result any `json:"result" jsonschema:"nano-gpt's Firecrawl response, passed through unchanged"`
}

type firecrawlScrapeInput struct {
	URL              string   `json:"url" jsonschema:"page to fetch"`
	Formats          []string `json:"formats,omitempty" jsonschema:"output formats to render, such as markdown or html; Firecrawl also serves rawHtml, links, and screenshots"`
	Proxy            string   `json:"proxy,omitempty" jsonschema:"basic uses Firecrawl's standard proxy; stealth costs more and gets past harder anti-bot defences"`
	OnlyMainContent  *bool    `json:"onlyMainContent,omitempty" jsonschema:"drop navigation, footers, and other boilerplate from the page"`
	OnlyCleanContent *bool    `json:"onlyCleanContent,omitempty" jsonschema:"run nano-gpt's extra cleanup pass over the extracted text"`
}

type firecrawlMapInput struct {
	URL     string `json:"url" jsonschema:"site to discover URLs on"`
	Sitemap string `json:"sitemap,omitempty" jsonschema:"include reads the site's sitemap alongside crawled links, skip ignores it, only uses it alone"`
	Limit   int    `json:"limit,omitempty" jsonschema:"maximum URLs to return"`
}

type firecrawlCrawlInput struct {
	URL               string   `json:"url" jsonschema:"site to crawl from"`
	Formats           []string `json:"formats,omitempty" jsonschema:"output formats to render, such as markdown or html; Firecrawl also serves rawHtml, links, and screenshots"`
	Proxy             string   `json:"proxy,omitempty" jsonschema:"basic uses Firecrawl's standard proxy; stealth costs more and gets past harder anti-bot defences"`
	OnlyMainContent   *bool    `json:"onlyMainContent,omitempty" jsonschema:"drop navigation, footers, and other boilerplate from each page"`
	OnlyCleanContent  *bool    `json:"onlyCleanContent,omitempty" jsonschema:"run nano-gpt's extra cleanup pass over the extracted text"`
	Sitemap           string   `json:"sitemap,omitempty" jsonschema:"include reads the site's sitemap alongside crawled links, skip ignores it, only uses it alone"`
	Limit             int      `json:"limit,omitempty" jsonschema:"maximum pages to crawl"`
	CrawlEntireDomain *bool    `json:"crawlEntireDomain,omitempty" jsonschema:"follow links across the whole domain, not just under the starting path"`
	AllowSubdomains   *bool    `json:"allowSubdomains,omitempty" jsonschema:"also crawl subdomains of the starting URL"`
	MaxDiscoveryDepth int      `json:"maxDiscoveryDepth,omitempty" jsonschema:"how many link hops from the starting URL to follow"`
	WaitForFinishSecs int      `json:"waitForFinishSecs,omitempty" jsonschema:"how long nano-gpt waits for the crawl before answering: 120 by default, 300 at most"`
	MaxReturnedPages  int      `json:"maxReturnedPages,omitempty" jsonschema:"how many crawled pages to return once the crawl settles: 25 by default"`
}

func registerFirecrawlScrape(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "firecrawl_scrape",
		Description: "Fetch one page through Firecrawl, with a proxy tier and content filtering that web_scrape " +
			"does not offer. Billed as Firecrawl credits through nano-gpt, rather than per URL.",
	}, h.firecrawlScrape)
}

func registerFirecrawlMap(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "firecrawl_map",
		Description: "List the URLs Firecrawl can discover on a site, without fetching their content. " +
			"Billed as Firecrawl credits through nano-gpt.",
	}, h.firecrawlMap)
}

func registerFirecrawlCrawl(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "firecrawl_crawl",
		Description: "Crawl a site from a starting URL and return the pages Firecrawl scraped. nano-gpt waits for " +
			"the crawl for up to waitForFinishSecs and returns up to maxReturnedPages of it. Each page is billed " +
			"as Firecrawl credits through nano-gpt.",
	}, h.firecrawlCrawl)
}

func (h *handlers) firecrawlScrape(ctx context.Context, _ *mcp.CallToolRequest, in firecrawlScrapeInput) (
	*mcp.CallToolResult, firecrawlOutput, error,
) {
	req := nanogpt.FirecrawlRequest{
		Operation:        nanogpt.OperationScrape,
		URL:              in.URL,
		Formats:          in.Formats,
		Proxy:            in.Proxy,
		OnlyMainContent:  in.OnlyMainContent,
		OnlyCleanContent: in.OnlyCleanContent,
	}

	return h.runFirecrawl(ctx, "firecrawl_scrape", req)
}

func (h *handlers) firecrawlMap(ctx context.Context, _ *mcp.CallToolRequest, in firecrawlMapInput) (
	*mcp.CallToolResult, firecrawlOutput, error,
) {
	req := nanogpt.FirecrawlRequest{
		Operation: nanogpt.OperationMap,
		URL:       in.URL,
		Sitemap:   in.Sitemap,
		Limit:     in.Limit,
	}

	return h.runFirecrawl(ctx, "firecrawl_map", req)
}

func (h *handlers) firecrawlCrawl(ctx context.Context, _ *mcp.CallToolRequest, in firecrawlCrawlInput) (
	*mcp.CallToolResult, firecrawlOutput, error,
) {
	req := nanogpt.FirecrawlRequest{
		Operation:         nanogpt.OperationCrawl,
		URL:               in.URL,
		Formats:           in.Formats,
		Proxy:             in.Proxy,
		OnlyMainContent:   in.OnlyMainContent,
		OnlyCleanContent:  in.OnlyCleanContent,
		Sitemap:           in.Sitemap,
		Limit:             in.Limit,
		CrawlEntireDomain: in.CrawlEntireDomain,
		AllowSubdomains:   in.AllowSubdomains,
		MaxDiscoveryDepth: in.MaxDiscoveryDepth,
		WaitForFinishSecs: in.WaitForFinishSecs,
		MaxReturnedPages:  in.MaxReturnedPages,
	}

	return h.runFirecrawl(ctx, "firecrawl_crawl", req)
}

func (h *handlers) runFirecrawl(ctx context.Context, tool string, req nanogpt.FirecrawlRequest) (
	*mcp.CallToolResult, firecrawlOutput, error,
) {
	result, err := h.nanogpt.Firecrawl(ctx, req)
	if err != nil {
		return nil, firecrawlOutput{}, h.fail(tool, err)
	}

	return nil, firecrawlOutput{Result: result}, nil
}
