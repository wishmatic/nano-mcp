package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

const scrapeUpstreamBody = `{
	"results": [
		{"url": "https://example.com", "success": true, "title": "Example", "markdown": "# Example"},
		{"url": "https://example.org", "success": false, "error": "blocked"}
	],
	"summary": {"stealthModeUsed": true, "successCount": 1}
}`

func TestWebScrapeReturnsEveryResult(t *testing.T) {
	var gotAuth string
	var gotBody nanogpt.ScrapeRequest

	baseURL := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(scrapeUpstreamBody))
	})

	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, baseURL)

	args := map[string]any{
		"urls":        []string{"https://example.com", "https://example.org"},
		"stealthMode": true,
	}

	result := callTool(t, connectedSession(t, deps), "web_scrape", args)
	if result.IsError {
		t.Fatalf("web_scrape failed: %s", errorText(result))
	}

	if gotAuth != "Bearer "+testNanoKey {
		t.Errorf("Authorization = %q, want the bearer key", gotAuth)
	}

	if len(gotBody.URLs) != 2 || !gotBody.StealthMode || gotBody.Provider != nanogpt.ProviderAuto {
		t.Errorf("upstream body = %+v, want both URLs, stealth mode, and the default provider", gotBody)
	}

	var out webScrapeOutput
	structured(t, result, &out)

	if len(out.Results) != 2 {
		t.Fatalf("results = %+v, want one entry per URL", out.Results)
	}

	if !out.Results[0].Success || out.Results[0].Markdown != "# Example" {
		t.Errorf("results[0] = %+v, want the scraped page", out.Results[0])
	}

	if out.Results[1].Success || out.Results[1].Error != "blocked" {
		t.Errorf("results[1] = %+v, want the per-URL failure", out.Results[1])
	}

	summary, ok := out.Summary.(map[string]any)
	if !ok || summary["stealthModeUsed"] != true {
		t.Errorf("summary = %#v, want the upstream stats kept whole", out.Summary)
	}
}

func TestWebScrapeRejectsURLCountsOutsideTheCap(t *testing.T) {
	var calls atomic.Int64

	baseURL := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	})

	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, baseURL)
	session := connectedSession(t, deps)

	cases := map[string][]string{
		"none": {},
		"six":  {"a", "b", "c", "d", "e", "f"},
	}

	for name, urls := range cases {
		t.Run(name, func(t *testing.T) {
			result := callTool(t, session, "web_scrape", map[string]any{"urls": urls})
			if !result.IsError {
				t.Fatal("IsError = false, want the URL count rejected")
			}

			if text := errorText(result); !strings.Contains(text, "1 to 5") {
				t.Errorf("error text = %q, want it to name the range", text)
			}
		})
	}

	if calls.Load() != 0 {
		t.Errorf("upstream calls = %d, want none for a rejected URL count", calls.Load())
	}
}
