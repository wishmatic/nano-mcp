package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

const searchUpstreamBody = `{
	"data": [{"title": "Go", "url": "https://go.dev"}],
	"metadata": {
		"query": "go",
		"provider": "linkup",
		"operation": "search",
		"depth": "standard",
		"outputType": "searchResults",
		"timestamp": "2026-10-03T12:00:00Z",
		"cost": 0.005
	}
}`

type bearerRoundTripper struct {
	token string
	base  http.RoundTripper
}

func (t bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)

	return t.base.RoundTrip(clone)
}

func connectedClient(t *testing.T, upstream http.HandlerFunc) *mcp.ClientSession {
	t.Helper()

	upstreamAPI := httptest.NewServer(upstream)
	t.Cleanup(upstreamAPI.Close)

	cfg := testConfig()
	cfg.NanoGPTBaseURL = upstreamAPI.URL

	srv, err := New(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	api := httptest.NewServer(srv.router)
	t.Cleanup(api.Close)

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).Connect(
		context.Background(),
		&mcp.StreamableClientTransport{
			Endpoint: api.URL + "/mcp",
			HTTPClient: &http.Client{Transport: bearerRoundTripper{
				token: cfg.APIKey,
				base:  http.DefaultTransport,
			}},
			DisableStandaloneSSE: true,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Connect() error: %v", err)
	}

	t.Cleanup(func() { _ = session.Close() })

	return session
}

func TestMCPOverHTTPListsTheTools(t *testing.T) {
	session := connectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the upstream was called for a listing: %s", r.URL.Path)
	})

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}

	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}

	slices.Sort(names)

	want := []string{
		"firecrawl_crawl", "firecrawl_map", "firecrawl_scrape", "web_scrape", "web_search", "youtube_transcribe",
	}

	if !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
	}
}

func TestMCPOverHTTPCallsWebSearch(t *testing.T) {
	session := connectedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/web" {
			t.Errorf("upstream path = %s, want /api/web", r.URL.Path)
		}

		if key := r.Header.Get("x-api-key"); key != "nano-key" {
			t.Errorf("x-api-key = %q, want the configured nano-gpt key", key)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchUpstreamBody))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "web_search",
		Arguments: map[string]any{"query": "go"},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}

	if result.IsError {
		t.Fatalf("web_search failed over HTTP: %#v", result.Content)
	}

	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}

	var out struct {
		Provider string  `json:"provider"`
		CostUSD  float64 `json:"costUsd"`
		Data     []any   `json:"data"`
	}

	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode structured content %s: %v", raw, err)
	}

	if out.Provider != "linkup" || out.CostUSD != 0.005 || len(out.Data) != 1 {
		t.Errorf("output = %+v, want the resolved provider, the cost, and one result", out)
	}
}

func TestMCPRejectsUnauthenticatedRequests(t *testing.T) {
	srv := newTestServer(t)

	api := httptest.NewServer(srv.router)
	t.Cleanup(api.Close)

	resp, err := http.Post(api.URL+"/mcp", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("Post() error: %v", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}
