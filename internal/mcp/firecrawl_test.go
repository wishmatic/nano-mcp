package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

const firecrawlUpstreamBody = `{"success": true, "data": {"markdown": "# Docs", "metadata": {"statusCode": 200}}}`

func firecrawlUpstream(t *testing.T, body string) (string, *nanogpt.FirecrawlRequest) {
	t.Helper()

	var got nanogpt.FirecrawlRequest

	baseURL := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	return baseURL, &got
}

func TestFirecrawlScrapeSendsTheOperationAndReturnsThePayload(t *testing.T) {
	baseURL, got := firecrawlUpstream(t, firecrawlUpstreamBody)

	args := map[string]any{
		"url":              "https://docs.firecrawl.dev",
		"formats":          []string{"markdown"},
		"proxy":            "basic",
		"onlyMainContent":  true,
		"onlyCleanContent": false,
	}

	result := callTool(t, nanoSession(t, baseURL), "firecrawl_scrape", args)
	if result.IsError {
		t.Fatalf("firecrawl_scrape failed: %s", errorText(result))
	}

	if got.Operation != nanogpt.OperationScrape || got.URL != "https://docs.firecrawl.dev" {
		t.Errorf("upstream body = %+v, want the scrape operation for the requested URL", got)
	}

	if len(got.Formats) != 1 || got.Formats[0] != "markdown" || got.Proxy != "basic" {
		t.Errorf("upstream body = %+v, want the formats and proxy passed through", got)
	}

	if got.OnlyMainContent == nil || !*got.OnlyMainContent {
		t.Errorf("onlyMainContent = %v, want an explicit true", got.OnlyMainContent)
	}

	if got.OnlyCleanContent == nil || *got.OnlyCleanContent {
		t.Errorf("onlyCleanContent = %v, want an explicit false", got.OnlyCleanContent)
	}

	var out firecrawlOutput
	structured(t, result, &out)

	payload, ok := out.Result.(map[string]any)
	if !ok || payload["success"] != true {
		t.Fatalf("result = %#v, want the upstream response", out.Result)
	}

	if data, ok := payload["data"].(map[string]any); !ok || data["markdown"] != "# Docs" {
		t.Errorf("data = %#v, want the nested page payload kept", payload["data"])
	}
}

func TestFirecrawlMapSendsTheOperationAndLeavesScrapeFieldsOut(t *testing.T) {
	baseURL, got := firecrawlUpstream(t, `{"success": true, "links": [{"url": "https://docs.firecrawl.dev"}]}`)

	args := map[string]any{"url": "https://docs.firecrawl.dev", "sitemap": "include", "limit": 10}

	result := callTool(t, nanoSession(t, baseURL), "firecrawl_map", args)
	if result.IsError {
		t.Fatalf("firecrawl_map failed: %s", errorText(result))
	}

	if got.Operation != nanogpt.OperationMap || got.Sitemap != "include" || got.Limit != 10 {
		t.Errorf("upstream body = %+v, want the map operation with its sitemap and limit", got)
	}

	if got.Formats != nil || got.Proxy != "" || got.OnlyMainContent != nil || got.WaitForFinishSecs != 0 {
		t.Errorf("upstream body = %+v, want the scrape and crawl fields unset", got)
	}
}

func TestFirecrawlCrawlSendsTheOperationAndDefaultsTheWait(t *testing.T) {
	baseURL, got := firecrawlUpstream(t, firecrawlUpstreamBody)

	args := map[string]any{
		"url":               "https://docs.firecrawl.dev",
		"onlyCleanContent":  false,
		"crawlEntireDomain": false,
		"allowSubdomains":   true,
		"maxDiscoveryDepth": 2,
		"limit":             10,
	}

	result := callTool(t, nanoSession(t, baseURL), "firecrawl_crawl", args)
	if result.IsError {
		t.Fatalf("firecrawl_crawl failed: %s", errorText(result))
	}

	if got.Operation != nanogpt.OperationCrawl || got.MaxDiscoveryDepth != 2 || got.Limit != 10 {
		t.Errorf("upstream body = %+v, want the crawl operation with its discovery knobs", got)
	}

	if got.WaitForFinishSecs != nanogpt.WaitForFinishDefault {
		t.Errorf("waitForFinishSecs = %d, want the %d default", got.WaitForFinishSecs, nanogpt.WaitForFinishDefault)
	}

	if got.MaxReturnedPages != nanogpt.MaxReturnedPagesDefault {
		t.Errorf("maxReturnedPages = %d, want the %d default", got.MaxReturnedPages, nanogpt.MaxReturnedPagesDefault)
	}

	if got.CrawlEntireDomain == nil || *got.CrawlEntireDomain {
		t.Errorf("crawlEntireDomain = %v, want an explicit false", got.CrawlEntireDomain)
	}

	if got.AllowSubdomains == nil || !*got.AllowSubdomains {
		t.Errorf("allowSubdomains = %v, want an explicit true", got.AllowSubdomains)
	}
}

func TestFirecrawlCrawlSendsTheRequestedWait(t *testing.T) {
	baseURL, got := firecrawlUpstream(t, firecrawlUpstreamBody)

	args := map[string]any{"url": "https://docs.firecrawl.dev", "waitForFinishSecs": 300, "maxReturnedPages": 5}

	result := callTool(t, nanoSession(t, baseURL), "firecrawl_crawl", args)
	if result.IsError {
		t.Fatalf("firecrawl_crawl failed: %s", errorText(result))
	}

	if got.WaitForFinishSecs != nanogpt.MaxWaitForFinishSecs || got.MaxReturnedPages != 5 {
		t.Errorf("upstream body = %+v, want the requested wait at the cap", got)
	}
}

func TestFirecrawlCrawlRejectsAWaitBeyondTheCap(t *testing.T) {
	var calls atomic.Int64

	baseURL := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	})

	args := map[string]any{"url": "https://docs.firecrawl.dev", "waitForFinishSecs": 301}

	result := callTool(t, nanoSession(t, baseURL), "firecrawl_crawl", args)
	if !result.IsError {
		t.Fatal("IsError = false, want a wait beyond the cap rejected")
	}

	if text := errorText(result); !strings.Contains(text, "1 and 300") {
		t.Errorf("error text = %q, want it to name the range", text)
	}

	if calls.Load() != 0 {
		t.Errorf("upstream calls = %d, want none for a rejected wait", calls.Load())
	}
}

func TestFirecrawlReportsUpstreamFailures(t *testing.T) {
	baseURL := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"firecrawl exploded"}`))
	})

	args := map[string]any{"url": "https://docs.firecrawl.dev"}

	result := callTool(t, nanoSession(t, baseURL), "firecrawl_scrape", args)
	if !result.IsError {
		t.Fatalf("IsError = false, want an upstream 500 to fail the call: %s", errorText(result))
	}

	if text := errorText(result); !strings.Contains(text, "500") {
		t.Errorf("error text = %q, want it to name the status", text)
	}
}
