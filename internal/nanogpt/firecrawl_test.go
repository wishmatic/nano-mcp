package nanogpt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func boolPtr(v bool) *bool {
	return &v
}

func firecrawlBody(t *testing.T, req FirecrawlRequest) *recordedRequest {
	t.Helper()

	srv, rec := recordingServer(t, `{"success": true}`, http.StatusOK)

	if _, err := newTestClient(t, srv.URL).Firecrawl(context.Background(), req); err != nil {
		t.Fatalf("Firecrawl() error: %v", err)
	}

	rec.assertHeader(t, "Content-Type", "application/json")
	rec.assertHeader(t, "Authorization", "Bearer "+testKey)

	if rec.method != http.MethodPost {
		t.Errorf("method = %s, want POST", rec.method)
	}

	if rec.path != "/api/v1/firecrawl" {
		t.Errorf("path = %s, want /api/v1/firecrawl", rec.path)
	}

	return rec
}

func TestFirecrawlScrapeSendsTheDocumentedBody(t *testing.T) {
	rec := firecrawlBody(t, FirecrawlRequest{
		Operation:        OperationScrape,
		URL:              "https://docs.firecrawl.dev",
		Formats:          []string{"markdown"},
		Proxy:            "basic",
		OnlyMainContent:  boolPtr(true),
		OnlyCleanContent: boolPtr(false),
	})

	rec.assertBody(t, "operation", OperationScrape)
	rec.assertBody(t, "url", "https://docs.firecrawl.dev")
	rec.assertBody(t, "proxy", "basic")
	rec.assertBody(t, "onlyMainContent", true)
	rec.assertBody(t, "onlyCleanContent", false)

	formats, ok := rec.body["formats"].([]any)
	if !ok || len(formats) != 1 || formats[0] != "markdown" {
		t.Errorf("formats = %#v, want [markdown]", rec.body["formats"])
	}

	for _, crawlOnly := range []string{"sitemap", "limit", "waitForFinishSecs", "maxReturnedPages", "crawlEntireDomain"} {
		rec.assertBodyOmits(t, crawlOnly)
	}
}

func TestFirecrawlMapSendsTheDocumentedBody(t *testing.T) {
	rec := firecrawlBody(t, FirecrawlRequest{
		Operation: OperationMap,
		URL:       "https://docs.firecrawl.dev",
		Sitemap:   "include",
		Limit:     10,
	})

	rec.assertBody(t, "operation", OperationMap)
	rec.assertBody(t, "url", "https://docs.firecrawl.dev")
	rec.assertBody(t, "sitemap", "include")
	rec.assertBodyNumber(t, "limit", 10)

	for _, scrapeOnly := range []string{"formats", "proxy", "onlyMainContent", "onlyCleanContent", "waitForFinishSecs"} {
		rec.assertBodyOmits(t, scrapeOnly)
	}
}

func TestFirecrawlCrawlSendsTheDocumentedBody(t *testing.T) {
	rec := firecrawlBody(t, FirecrawlRequest{
		Operation:         OperationCrawl,
		URL:               "https://docs.firecrawl.dev",
		Formats:           []string{"markdown"},
		Proxy:             "basic",
		OnlyMainContent:   boolPtr(true),
		OnlyCleanContent:  boolPtr(false),
		Sitemap:           "include",
		Limit:             10,
		CrawlEntireDomain: boolPtr(false),
		AllowSubdomains:   boolPtr(true),
		MaxDiscoveryDepth: 2,
	})

	rec.assertBody(t, "operation", OperationCrawl)
	rec.assertBody(t, "url", "https://docs.firecrawl.dev")
	rec.assertBody(t, "proxy", "basic")
	rec.assertBody(t, "onlyMainContent", true)
	rec.assertBody(t, "onlyCleanContent", false)
	rec.assertBody(t, "sitemap", "include")
	rec.assertBodyNumber(t, "limit", 10)
	rec.assertBody(t, "crawlEntireDomain", false)
	rec.assertBody(t, "allowSubdomains", true)
	rec.assertBodyNumber(t, "maxDiscoveryDepth", 2)
	rec.assertBodyNumber(t, "waitForFinishSecs", WaitForFinishDefault)
	rec.assertBodyNumber(t, "maxReturnedPages", MaxReturnedPagesDefault)
}

func TestFirecrawlCrawlSendsTheRequestedWait(t *testing.T) {
	rec := firecrawlBody(t, FirecrawlRequest{
		Operation:         OperationCrawl,
		URL:               "https://docs.firecrawl.dev",
		WaitForFinishSecs: 45,
		MaxReturnedPages:  5,
	})

	rec.assertBodyNumber(t, "waitForFinishSecs", 45)
	rec.assertBodyNumber(t, "maxReturnedPages", 5)
}

func TestFirecrawlOmitsUnsetBooleans(t *testing.T) {
	rec := firecrawlBody(t, FirecrawlRequest{Operation: OperationCrawl, URL: "https://example.com"})

	for _, unset := range []string{
		"onlyMainContent", "onlyCleanContent", "crawlEntireDomain", "allowSubdomains",
	} {
		rec.assertBodyOmits(t, unset)
	}
}

func TestFirecrawlBudgetsStayWithinTheCap(t *testing.T) {
	crawlDefault := time.Duration(WaitForFinishDefault)*time.Second + crawlSlack

	cases := []struct {
		name string
		req  FirecrawlRequest
		want time.Duration
	}{
		{"scrape", FirecrawlRequest{Operation: OperationScrape, URL: "u"}, FastTimeout},
		{"map", FirecrawlRequest{Operation: OperationMap, URL: "u"}, FastTimeout},
		{"crawl at the cap", FirecrawlRequest{Operation: OperationCrawl, URL: "u", WaitForFinishSecs: MaxWaitForFinishSecs}, MaxCallBudget},
		{"crawl by default", FirecrawlRequest{Operation: OperationCrawl, URL: "u"}, crawlDefault},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			budget, err := tt.req.prepare()
			if err != nil {
				t.Fatalf("prepare() error: %v", err)
			}

			if budget != tt.want {
				t.Errorf("budget = %s, want %s", budget, tt.want)
			}

			if budget > MaxCallBudget {
				t.Errorf("budget = %s, want it within MaxCallBudget (%s)", budget, MaxCallBudget)
			}
		})
	}
}

func TestFirecrawlReturnsThePayloadWhole(t *testing.T) {
	body := `{"success": true, "data": {"markdown": "# Docs", "metadata": {"statusCode": 200}}, "links": ["a", "b"]}`
	srv, _ := recordingServer(t, body, http.StatusOK)

	got, err := newTestClient(t, srv.URL).Firecrawl(context.Background(), FirecrawlRequest{
		Operation: OperationScrape,
		URL:       "https://docs.firecrawl.dev",
	})
	if err != nil {
		t.Fatalf("Firecrawl() error: %v", err)
	}

	payload, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("payload = %#v, want the decoded response object", got)
	}

	data, ok := payload["data"].(map[string]any)
	if !ok || data["markdown"] != "# Docs" {
		t.Errorf("data = %#v, want the nested page payload", payload["data"])
	}

	if _, ok := payload["links"].([]any); !ok {
		t.Errorf("links = %#v, want the array kept", payload["links"])
	}
}

func TestFirecrawlRejectsOutOfRangeValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a request was sent for arguments the endpoint cannot accept")
	}))
	t.Cleanup(srv.Close)

	cases := map[string]struct {
		req      FirecrawlRequest
		wantWord string
	}{
		"wait too long":  {FirecrawlRequest{Operation: OperationCrawl, URL: "u", WaitForFinishSecs: 301}, "1 and 300"},
		"negative limit": {FirecrawlRequest{Operation: OperationMap, URL: "u", Limit: -1}, "limit"},
		"negative depth": {FirecrawlRequest{Operation: OperationCrawl, URL: "u", MaxDiscoveryDepth: -1}, "maxDiscoveryDepth"},
		"negative pages": {FirecrawlRequest{Operation: OperationCrawl, URL: "u", MaxReturnedPages: -1}, "maxReturnedPages"},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newTestClient(t, srv.URL).Firecrawl(context.Background(), tt.req)
			if err == nil {
				t.Fatal("Firecrawl() error = nil, want the argument rejected")
			}

			if !strings.Contains(err.Error(), tt.wantWord) {
				t.Errorf("error = %q, want it to name %q", err, tt.wantWord)
			}
		})
	}
}

func TestFirecrawlReportsUpstreamFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, _ := recordingServer(t, `{"error":"firecrawl said no"}`, status)

			_, err := newTestClient(t, srv.URL).Firecrawl(context.Background(), FirecrawlRequest{
				Operation: OperationMap,
				URL:       "https://example.com",
			})
			if err == nil || !strings.Contains(err.Error(), strconv.Itoa(status)) {
				t.Errorf("Firecrawl() error = %v, want it to name the status", err)
			}

			if err != nil && !strings.Contains(err.Error(), "firecrawl said no") {
				t.Errorf("error = %q, want the body quoted", err)
			}
		})
	}
}

func TestFirecrawlRejectsAMalformedResponse(t *testing.T) {
	srv, _ := recordingServer(t, `{"success": true`, http.StatusOK)

	_, err := newTestClient(t, srv.URL).Firecrawl(context.Background(), FirecrawlRequest{
		Operation: OperationMap,
		URL:       "https://example.com",
	})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("Firecrawl() error = %v, want a decode error", err)
	}
}
