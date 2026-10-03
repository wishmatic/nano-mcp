# Firecrawl tools

Status: Done
Depends on: `done/WEB_TOOLS_PLAN.md` (the `internal/nanogpt` client and the two web tools)

## Goal

Expose nano-gpt's Firecrawl adapter, `POST /api/v1/firecrawl`, as three MCP tools: `firecrawl_scrape`,
`firecrawl_map`, and `firecrawl_crawl`. One endpoint serves all three operations, chosen by an `operation` field in
the body.

## Does `firecrawl_scrape` earn its place?

`web_scrape` already fetches page text, so the scrape operation has to be worth a second tool. It is, on three
counts:

1. **Different provider, different bill.** `web_scrape` is nano-gpt's own per-URL scraping at about 0.0015 per URL.
   Firecrawl is a partner endpoint billed in Firecrawl credits with a nano-gpt markup, so the two are not
   interchangeable when cost matters.
2. **Controls `web_scrape` does not have.** Firecrawl takes a proxy tier plus `onlyMainContent` and
   `onlyCleanContent` filtering, and a `formats` list. `web_scrape` takes only `stealthMode` and a provider choice.
3. **Different payload.** `web_scrape` normalises to one entry per URL with a request summary. Firecrawl returns its
   own response, which carries its own metadata and page structures.

What it does not earn is a new transport or a new tool shape; all three operations share one client method and one
output envelope.

## Non-goals

- nano-gpt's other data endpoints (`/api/v1/reddit`, googlemaps, hunter, and the rest of the Data API catalog), and
  the `/api/v1/data/firecrawl` dispatch path, which wraps the same endpoint.
- Firecrawl's own extras: actions, schemas, JSON extraction, webhooks, pagination of large crawls via `next`, and
  async job polling. nano-gpt's `waitForFinishSecs` and `maxReturnedPages` exist to avoid the last of those.
- Typed modelling of the Firecrawl response (see Design -> Response).
- Retrying, caching, or spend accounting.

## Design

### One endpoint, three operations

| Tool                 | `operation` | Extra fields                                                        |
| -------------------- | ----------- | ------------------------------------------------------------------- |
| `firecrawl_scrape`   | `scrape`    | `formats`, `proxy`, `onlyMainContent`, `onlyCleanContent`            |
| `firecrawl_map`      | `map`       | `sitemap`, `limit`                                                   |
| `firecrawl_crawl`    | `crawl`     | everything above plus the crawl discovery and wait knobs             |

`operation` is set by each handler and is not an input field; the tool name already names it.

### Auth

`Authorization: Bearer <key>`, matching the Data API's own curl examples for its `/api/v1/*` endpoints. The
scrape-urls page lists both `apiKeyAuth` and `bearerAuth` and its examples use `x-api-key`, so the two families
disagree; this is the one request-side choice that no public documentation confirms for this endpoint, and the live
check covers it.

### Request types

`nanogpt.FirecrawlRequest` mirrors the wire body with one field per parameter, `omitempty` throughout, and one
method, `Firecrawl(ctx, req) (any, error)`.

Four booleans are `*bool`: `onlyMainContent`, `onlyCleanContent`, `crawlEntireDomain`, and `allowSubdomains`. Their
upstream defaults are undocumented, so unset and `false` must stay distinguishable. `web_scrape`'s `stealthMode`
stays a plain `bool` because `false` is its documented default, so omitting it loses nothing.

Numbers keep `0` as "unset": `limit`, `maxDiscoveryDepth`, `waitForFinishSecs`, and `maxReturnedPages` are all
meaningless at zero, so `omitempty` is unambiguous.

### Response

The response is passed through as opaque JSON in a one-field envelope:

```go
type firecrawlOutput struct {
	Result any `json:"result" jsonschema:"nano-gpt's Firecrawl response, passed through unchanged"`
}
```

NanoGPT documents this endpoint's inputs (through the request shapes) but no response shape or example, and Firecrawl
documents its own v1 responses for three operations that nano-gpt's adapter may reshape. A typed model built from
that guesswork would decode unknown keys to zero values and silently drop the rest, which is exactly how the
`cost`/`costUsd` mistake in the web tools plan survived a passing test suite. `any` infers an unrestricted schema, so
the payload reaches the model whole.

### Timeouts

A crawl asks nano-gpt to wait, so its budget has to follow the requested wait, and the transport can no longer be
bounded by one flat client timeout:

- `FastTimeout` (90s) bounds `web_search`, `web_scrape`, and firecrawl's `scrape` and `map`.
- A `crawl` gets `waitForFinishSecs + 30s` so nano-gpt has room to assemble its answer.
- `waitForFinishSecs` is capped at `MaxWaitForFinishSecs` (300s) and defaults to 120, matching the documented request
  shape. The cap exists so `MaxCallBudget`, 300s plus the 30s slack, is a real ceiling.
- `post` applies the budget as a context deadline per request and `http.Client` carries no timeout of its own, so a
  search still fails in 90s rather than waiting out a crawl's budget.
- `internal/server`'s write timeout moves from 120s to 360s so it clears `MaxCallBudget`, and the existing test
  asserts that ordering against the new constant.

## Implementation units

### Unit 1: the Firecrawl client

Deliverables: `internal/nanogpt/firecrawl.go` with `firecrawl_test.go`, and the per-request budget change to
`internal/nanogpt/client.go` with its test.

Acceptance criteria:

- [x] Tests over `httptest` assert `POST /api/v1/firecrawl`, the bearer header, and one body per operation: `scrape`
      carries `formats`, `proxy`, and the content filters; `map` carries `sitemap` and `limit`; `crawl` carries every
      documented field.
- [x] An unset `*bool` is omitted from the body while an explicit `false` is sent, for all four tri-state booleans.
- [x] A crawl defaults `waitForFinishSecs` to 120 and `maxReturnedPages` to 25 when unset, and sends them when set.
- [x] `waitForFinishSecs` outside 1 to 300 is rejected without a request, naming the range; `maxReturnedPages` below
      1 and negative `limit` or `maxDiscoveryDepth` are rejected likewise.
- [x] The response decodes whole: object, nested object, and array payloads survive unchanged.
- [x] 401, 429, and 500 produce an error naming the status, and malformed JSON a decode error, for this endpoint too.
- [x] `MaxCallBudget` equals `MaxWaitForFinishSecs` plus the crawl slack, and no call's budget exceeds it.
- [x] `go build ./...`, `go vet ./...`, and `go test ./... -race -count=1` pass.

### Unit 2: the tool surface and server wiring

Deliverables: `internal/mcp/firecrawl.go` with `firecrawl_test.go`, updated `internal/mcp/server.go`, updated
`internal/server/server.go` with `server_test.go` and `http_test.go`.

Acceptance criteria:

- [x] `tools/list` is exactly the five tools, sorted, and still empty when no client is configured.
- [x] Session tests drive all three operations against an `httptest` Firecrawl and assert the body each one sends and
      the payload each one returns.
- [x] A crawl with no wait given sends 120 seconds, and one asking for more than the cap fails without a request.
- [x] An upstream 500 reaches the client as a failed tool call naming the status.
- [x] `server.New` still refuses to start without `NANOGPT_API_KEY`, and its write timeout clears `MaxCallBudget`,
      asserted by a test.
- [x] The web tools' existing tests pass unchanged.
- [x] `go build ./...`, `go vet ./...`, and `go test ./... -race -count=1` pass.

### Unit 3: documentation

Deliverables: `README.md`, this plan.

Acceptance criteria:

- [x] `README.md` stays short and lists the three tools with their endpoint, and says in one line how
      `firecrawl_scrape` differs from `web_scrape`.
- [ ] Human check: with a real key, all three operations answer, which is also what confirms the bearer header, the
      crawl wait, and the response shape.

## Verification

Per unit: `go build ./...`, `go vet ./...`, `go test ./... -race -count=1`, and `gofmt -l .`. The criteria that matter
most are the per-operation request bodies, the tri-state booleans, and the budget ordering, because a wrong body
field or a silent `false` is invisible in a green test run.
