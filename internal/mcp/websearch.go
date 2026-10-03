package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

type webSearchInput struct {
	Query      string `json:"query" jsonschema:"what to search for"`
	Provider   string `json:"provider,omitempty" jsonschema:"tavily, brave, linkup, exa, kagi, perplexity, valyu, sofya, or firecrawl; nano-gpt routes when unset"`
	Depth      string `json:"depth,omitempty" jsonschema:"search depth, standard unless deepened"`
	OutputType string `json:"outputType,omitempty" jsonschema:"searchResults returns a normalized result array, sourcedAnswer and structured return the provider's own answer object"`
}

type webSearchOutput struct {
	Query      string  `json:"query" jsonschema:"query that was run"`
	Provider   string  `json:"provider" jsonschema:"provider that answered"`
	Operation  string  `json:"operation" jsonschema:"operation performed, search except for the Sofya fetch, extract, and research requests"`
	Depth      string  `json:"depth" jsonschema:"depth that was used"`
	OutputType string  `json:"outputType" jsonschema:"output type that was returned"`
	Timestamp  string  `json:"timestamp" jsonschema:"ISO-8601 timestamp of the request"`
	CostUSD    float64 `json:"costUsd" jsonschema:"what nano-gpt charged for the request, in USD"`
	Data       any     `json:"data" jsonschema:"normalized results for searchResults, otherwise the provider's response object"`
}

func registerWebSearch(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "web_search",
		Description: "Search the web through nano-gpt's providers. Costs money per request; the charge is " +
			"reported in costUsd. Returns citations and page content rather than the pages themselves.",
	}, h.webSearch)
}

func (h *handlers) webSearch(ctx context.Context, _ *mcp.CallToolRequest, in webSearchInput) (
	*mcp.CallToolResult, webSearchOutput, error,
) {
	req := nanogpt.SearchRequest{
		Query:      in.Query,
		Provider:   in.Provider,
		Depth:      in.Depth,
		OutputType: in.OutputType,
	}

	resp, err := h.nanogpt.Search(ctx, req)
	if err != nil {
		return nil, webSearchOutput{}, h.fail("web_search", err)
	}

	return nil, webSearchOutput{
		Query:      resp.Metadata.Query,
		Provider:   resp.Metadata.Provider,
		Operation:  resp.Metadata.Operation,
		Depth:      resp.Metadata.Depth,
		OutputType: resp.Metadata.OutputType,
		Timestamp:  resp.Metadata.Timestamp,
		CostUSD:    resp.Metadata.CostUSD,
		Data:       resp.Data,
	}, nil
}
