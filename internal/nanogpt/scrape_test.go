package nanogpt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const scrapeResponseBody = `{
	"results": [
		{"url": "https://example.com", "success": true, "title": "Example", "markdown": "# Example", "content": "Example text"},
		{"url": "https://example.org", "success": false, "error": "blocked"}
	],
	"summary": {"stealthModeUsed": false, "successCount": 1}
}`

func TestScrapeSendsTheBearerKeyAndDefaults(t *testing.T) {
	srv, rec := recordingServer(t, scrapeResponseBody, http.StatusOK)

	req := ScrapeRequest{URLs: []string{"https://example.com"}}
	if _, err := newTestClient(t, srv.URL).Scrape(context.Background(), req); err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}

	rec.assertHeader(t, "Content-Type", "application/json")
	rec.assertHeader(t, "Authorization", "Bearer "+testKey)
	rec.assertHeader(t, "x-api-key", "")

	if rec.method != http.MethodPost {
		t.Errorf("method = %s, want POST", rec.method)
	}

	if rec.path != "/api/scrape-urls" {
		t.Errorf("path = %s, want /api/scrape-urls", rec.path)
	}

	rec.assertBody(t, "provider", ProviderAuto)
	rec.assertBodyOmits(t, "stealthMode")

	urls, ok := rec.body["urls"].([]any)
	if !ok || len(urls) != 1 || urls[0] != "https://example.com" {
		t.Errorf("urls = %#v, want the requested URL", rec.body["urls"])
	}
}

func TestScrapeSendsStealthModeOnlyWhenSet(t *testing.T) {
	srv, rec := recordingServer(t, scrapeResponseBody, http.StatusOK)

	req := ScrapeRequest{URLs: []string{"https://example.com"}, StealthMode: true, Provider: "linkup"}
	if _, err := newTestClient(t, srv.URL).Scrape(context.Background(), req); err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}

	rec.assertBody(t, "stealthMode", true)
	rec.assertBody(t, "provider", "linkup")
}

func TestScrapeDecodesTheResponse(t *testing.T) {
	srv, _ := recordingServer(t, scrapeResponseBody, http.StatusOK)

	resp, err := newTestClient(t, srv.URL).Scrape(context.Background(), ScrapeRequest{URLs: []string{"https://example.com"}})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("results = %+v, want one entry per URL", resp.Results)
	}

	if !resp.Results[0].Success || resp.Results[0].Markdown != "# Example" {
		t.Errorf("results[0] = %+v, want the scraped page", resp.Results[0])
	}

	if resp.Results[1].Success || resp.Results[1].Error != "blocked" {
		t.Errorf("results[1] = %+v, want the per-URL failure", resp.Results[1])
	}

	summary, ok := resp.Summary.(map[string]any)
	if !ok || summary["stealthModeUsed"] != false {
		t.Errorf("summary = %#v, want the upstream stats kept whole", resp.Summary)
	}
}

func TestScrapeRejectsURLCountsOutsideTheCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a request was sent for a URL count the API does not accept")
	}))
	t.Cleanup(srv.Close)

	cases := map[string][]string{
		"none": {},
		"six":  {"a", "b", "c", "d", "e", "f"},
	}

	for name, urls := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newTestClient(t, srv.URL).Scrape(context.Background(), ScrapeRequest{URLs: urls})
			if err == nil || !strings.Contains(err.Error(), "1 to 5") {
				t.Errorf("Scrape() error = %v, want the 1 to 5 range named", err)
			}
		})
	}
}
