package nanogpt

import "context"

const (
	searchPath = "/api/web"

	// These mirror NanoGPT's own defaults, applied here so every request states what it
	// asked for and the resolved values in the response can be compared against them.
	DepthDefault      = "standard"
	OutputTypeDefault = "searchResults"
)

type SearchRequest struct {
	Query      string `json:"query"`
	Depth      string `json:"depth,omitempty"`
	Provider   string `json:"provider,omitempty"`
	OutputType string `json:"outputType,omitempty"`
}

type SearchResponse struct {
	// Data is an array of normalized results for searchResults output, and the provider's
	// own response object for sourcedAnswer, structured, and the Sofya operations.
	Data     any      `json:"data"`
	Metadata Metadata `json:"metadata"`
}

type Metadata struct {
	Query      string  `json:"query"`
	Provider   string  `json:"provider"`
	Operation  string  `json:"operation"`
	Depth      string  `json:"depth"`
	OutputType string  `json:"outputType"`
	Timestamp  string  `json:"timestamp"`
	CostUSD    float64 `json:"cost"`
}

func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if req.Depth == "" {
		req.Depth = DepthDefault
	}
	if req.OutputType == "" {
		req.OutputType = OutputTypeDefault
	}

	var out SearchResponse

	cred := credential{header: "x-api-key", value: c.apiKey}
	if err := c.post(ctx, searchPath, FastTimeout, cred, req, &out); err != nil {
		return nil, err
	}

	return &out, nil
}
