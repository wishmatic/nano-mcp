package nanogpt

import (
	"context"
	"fmt"
	"time"
)

const (
	firecrawlPath = "/api/v1/firecrawl"

	OperationScrape = "scrape"
	OperationMap    = "map"
	OperationCrawl  = "crawl"

	WaitForFinishDefault    = 120
	MaxReturnedPagesDefault = 25

	// MaxWaitForFinishSecs caps how long a crawl may ask nano-gpt to wait, so every call
	// fits inside MaxCallBudget and therefore inside internal/server's write timeout.
	MaxWaitForFinishSecs = 300

	crawlSlack    = 30 * time.Second
	MaxCallBudget = time.Duration(MaxWaitForFinishSecs)*time.Second + crawlSlack
)

// FirecrawlRequest mirrors the body this endpoint takes. It is one request for all three
// operations, since only the fields a given operation reads differ.
type FirecrawlRequest struct {
	Operation string `json:"operation"`

	URL     string   `json:"url"`
	Formats []string `json:"formats,omitempty"`
	Proxy   string   `json:"proxy,omitempty"`

	// Pointers because nano-gpt documents no defaults for these, so unset has to stay
	// distinguishable from an explicit false.
	OnlyMainContent   *bool `json:"onlyMainContent,omitempty"`
	OnlyCleanContent  *bool `json:"onlyCleanContent,omitempty"`
	CrawlEntireDomain *bool `json:"crawlEntireDomain,omitempty"`
	AllowSubdomains   *bool `json:"allowSubdomains,omitempty"`

	Sitemap           string `json:"sitemap,omitempty"`
	Limit             int    `json:"limit,omitempty"`
	MaxDiscoveryDepth int    `json:"maxDiscoveryDepth,omitempty"`

	WaitForFinishSecs int `json:"waitForFinishSecs,omitempty"`
	MaxReturnedPages  int `json:"maxReturnedPages,omitempty"`
}

// Firecrawl returns the response body decoded but unmodelled: nano-gpt documents no shape
// for it, and the three operations do not share one.
func (c *Client) Firecrawl(ctx context.Context, req FirecrawlRequest) (any, error) {
	budget, err := req.prepare()
	if err != nil {
		return nil, err
	}

	var out any

	cred := credential{header: "Authorization", value: "Bearer " + c.apiKey}
	if err := c.post(ctx, firecrawlPath, budget, cred, req, &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (r *FirecrawlRequest) prepare() (time.Duration, error) {
	if err := r.checkNumbers(); err != nil {
		return 0, err
	}

	if r.Operation != OperationCrawl {
		return FastTimeout, nil
	}

	if r.WaitForFinishSecs == 0 {
		r.WaitForFinishSecs = WaitForFinishDefault
	}
	if r.WaitForFinishSecs < 1 || r.WaitForFinishSecs > MaxWaitForFinishSecs {
		return 0, fmt.Errorf(
			"waitForFinishSecs must be between 1 and %d, got %d", MaxWaitForFinishSecs, r.WaitForFinishSecs,
		)
	}

	if r.MaxReturnedPages == 0 {
		r.MaxReturnedPages = MaxReturnedPagesDefault
	}

	return time.Duration(r.WaitForFinishSecs)*time.Second + crawlSlack, nil
}

func (r *FirecrawlRequest) checkNumbers() error {
	if r.Limit < 0 {
		return fmt.Errorf("limit must not be negative, got %d", r.Limit)
	}
	if r.MaxDiscoveryDepth < 0 {
		return fmt.Errorf("maxDiscoveryDepth must not be negative, got %d", r.MaxDiscoveryDepth)
	}
	if r.MaxReturnedPages < 0 {
		return fmt.Errorf("maxReturnedPages must not be negative, got %d", r.MaxReturnedPages)
	}

	return nil
}
