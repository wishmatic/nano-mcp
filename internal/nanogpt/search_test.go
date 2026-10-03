package nanogpt

import (
	"context"
	"net/http"
	"testing"
)

const searchResponseBody = `{
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

func TestSearchSendsTheKeyAndDefaults(t *testing.T) {
	srv, rec := recordingServer(t, searchResponseBody, http.StatusOK)

	if _, err := newTestClient(t, srv.URL).Search(context.Background(), SearchRequest{Query: "go"}); err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	rec.assertHeader(t, "Content-Type", "application/json")
	rec.assertHeader(t, "x-api-key", testKey)
	rec.assertHeader(t, "Authorization", "")

	if rec.method != http.MethodPost {
		t.Errorf("method = %s, want POST", rec.method)
	}

	if rec.path != "/api/web" {
		t.Errorf("path = %s, want /api/web", rec.path)
	}

	rec.assertBody(t, "query", "go")
	rec.assertBody(t, "depth", DepthDefault)
	rec.assertBody(t, "outputType", OutputTypeDefault)
	rec.assertBodyOmits(t, "provider")
}

func TestSearchSendsEveryFieldWhenSet(t *testing.T) {
	srv, rec := recordingServer(t, searchResponseBody, http.StatusOK)

	req := SearchRequest{Query: "go", Depth: "deep", Provider: "tavily", OutputType: "sourcedAnswer"}
	if _, err := newTestClient(t, srv.URL).Search(context.Background(), req); err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	rec.assertBody(t, "depth", "deep")
	rec.assertBody(t, "provider", "tavily")
	rec.assertBody(t, "outputType", "sourcedAnswer")
}

func TestSearchDecodesTheResponse(t *testing.T) {
	srv, _ := recordingServer(t, searchResponseBody, http.StatusOK)

	resp, err := newTestClient(t, srv.URL).Search(context.Background(), SearchRequest{Query: "go"})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if resp.Metadata.Provider != "linkup" || resp.Metadata.Operation != "search" {
		t.Errorf("metadata = %+v, want the resolved provider and operation", resp.Metadata)
	}

	if resp.Metadata.CostUSD != 0.005 {
		t.Errorf("costUsd = %v, want 0.005", resp.Metadata.CostUSD)
	}

	results, ok := resp.Data.([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("data = %#v, want one normalized result", resp.Data)
	}

	first, ok := results[0].(map[string]any)
	if !ok || first["url"] != "https://go.dev" {
		t.Errorf("data[0] = %#v, want the provider's result", results[0])
	}
}
