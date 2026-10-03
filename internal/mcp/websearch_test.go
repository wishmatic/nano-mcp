package mcp

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

const searchUpstreamBody = `{
	"data": [{"title": "Go", "url": "https://go.dev", "content": "the language"}],
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

func TestWebSearchSchemaPinsTheProviderValues(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, "http://127.0.0.1:1")

	properties := toolProperties(t, findTool(t, connectedSession(t, deps), "web_search"))

	if got := properties["provider"].Enum; !slices.Equal(got, searchProviders) {
		t.Errorf("provider enum = %v, want %v", got, searchProviders)
	}

	if got := properties["outputType"].Enum; !slices.Equal(got, searchOutputTypes) {
		t.Errorf("outputType enum = %v, want %v", got, searchOutputTypes)
	}

	if description := properties["provider"].Description; strings.Contains(description, "valyu") {
		t.Errorf("provider description = %q, want the names left to the enum", description)
	}
}

func TestWebSearchReturnsTheResolvedRequestAndResults(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, jsonUpstream(t, "/api/web", searchUpstreamBody))

	result := callTool(t, connectedSession(t, deps), "web_search", map[string]any{"query": "go"})
	if result.IsError {
		t.Fatalf("web_search failed: %s", errorText(result))
	}

	var out webSearchOutput
	structured(t, result, &out)

	if out.Provider != "linkup" || out.Operation != "search" {
		t.Errorf("output = %+v, want the provider nano-gpt resolved", out)
	}

	if out.Depth != "standard" || out.OutputType != "searchResults" || out.Query != "go" {
		t.Errorf("output = %+v, want the defaults echoed back", out)
	}

	if out.CostUSD != 0.005 {
		t.Errorf("costUsd = %v, want 0.005", out.CostUSD)
	}

	results, ok := out.Data.([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("data = %#v, want one result", out.Data)
	}

	if first, ok := results[0].(map[string]any); !ok || first["url"] != "https://go.dev" {
		t.Errorf("data[0] = %#v, want the provider's result", results[0])
	}
}

func TestWebSearchReportsUpstreamFailures(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"provider exploded"}`))
	}))

	result := callTool(t, connectedSession(t, deps), "web_search", map[string]any{"query": "go"})
	if !result.IsError {
		t.Fatalf("IsError = false, want an upstream 500 to fail the call: %s", errorText(result))
	}

	if text := errorText(result); !strings.Contains(text, "500") || !strings.Contains(text, "provider exploded") {
		t.Errorf("error text = %q, want the status and body", text)
	}
}
