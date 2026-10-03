# Web tools

Status: Done
Depends on: none

## Goal

Expose NanoGPT's web search and URL scraping as two MCP tools, so an MCP client can search the web and read pages
through NanoGPT's provider routing and billing surface: `web_search` wraps `POST /api/web`, `web_scrape` wraps
`POST /api/scrape-urls`.

## Non-goals

- The rest of NanoGPT's data endpoints: `/api/v1/firecrawl`, `/api/v1/reddit`, YouTube transcription, and accountless
  x402 requests.
- Search fields beyond the four in scope: `operation`, `structuredOutputSchema`, `includeImages`, `fromDate`,
  `toDate`, `includeDomains`, `excludeDomains`, `limit`.
- Streaming, caching, retries, concurrency, or spend accounting beyond reporting the per-request cost the API
  returns.
- URL validation beyond what NanoGPT enforces: it already rejects non-HTTP(S), localhost, private IPs, metadata
  endpoints, and YouTube.
- Converting scraped output. NanoGPT returns formatted text; HTML is deliberately not offered upstream.

## Design

### Endpoints and the auth asymmetry

| Tool         | Path               | Auth header                   |
| ------------ | ------------------ | ----------------------------- |
| `web_search` | `/api/web`         | `x-api-key: <key>`            |
| `web_scrape` | `/api/scrape-urls` | `Authorization: Bearer <key>` |

Both endpoints accept either header, but each tool sends the one specified for it. Both send
`Content-Type: application/json`.

### Configuration

| Envar              | Default                | Meaning                                                               |
| ------------------ | ---------------------- | --------------------------------------------------------------------- |
| `NANOGPT_API_KEY`  | unset                  | Required. Key for both tools; the server refuses to start without it. |
| `NANOGPT_BASE_URL` | `https://nano-gpt.com` | Base URL. Overridden by tests, and by NanoGPT's alternative domains.  |

`NANOGPT_API_KEY` is required rather than optional because every tool this server exposes needs it; a missing key is
a deployment mistake that must fail at startup instead of at the first tool call. The base URL is configurable
because NanoGPT documents mirror domains (`ai.bitcoin.com`, `bcashgpt.com`, `cake.nano-gpt.com`) and because tests
need to point the client at `httptest`. The default lives in the client alone, so `NANOGPT_BASE_URL` is left empty
when unset rather than defaulted twice.

### Package graph

`internal/nanogpt` is a leaf like `internal/auth`: it imports only the standard library and never the MCP SDK.
`internal/mcp` imports it for tool handlers, and `internal/server` constructs it. The existing invariant holds: only
`internal/server` imports `internal/mcp`.

### Client

`internal/nanogpt` holds the transport, so tool handlers contain no HTTP:

- `Client` carries the key, base URL, and an `*http.Client` with a 90s timeout.
- `New(Config)` returns `ErrNoAPIKey` when the key is empty.
- `Search(ctx, SearchRequest) (*SearchResponse, error)` and `Scrape(ctx, ScrapeRequest) (*ScrapeResponse, error)`,
  both over one private `post` helper that marshals the body, sets the auth header, checks the status, and decodes.
- A non-2xx response becomes an error naming the endpoint, status, and a truncated body. A 200 that does not decode
  becomes a decode error.
- The client applies NanoGPT's defaults (`depth`, `outputType`, scrape `provider`), so a request always states what it
  is asking for and the resolved values in the response can be compared against it.

Errors are values, not `CallToolResult` mutations: the SDK sets `IsError` and puts the error text in the result, so a
failed upstream call reaches the model as a failed tool call with the reason.

### Schemas

`AddTool` infers both schemas from the handler types. Two constraints shape the surface:

1. The `jsonschema` struct tag carries a description only, and a field is required unless its `json` tag has
   `omitempty`. Allowed-value sets therefore live in descriptions; NanoGPT resolves the provider, depth, and output
   type and rejects what it does not support. Encoding them as Go enums is not possible here, so there is nothing to
   drift out of sync.
2. `data` in the search response and `summary` in the scrape response are typed `any` (an unrestricted schema). The
   search payload is an array of normalized results for `searchResults` but an object for `sourcedAnswer` and
   `structured`, and NanoGPT documents only part of the scrape summary. Modelling either as one struct would drop
   fields or reject valid upstream payloads.

`web_search` input: `query`, `depth` (default `standard`), `provider` (one of `tavily`, `brave`, `linkup`, `exa`,
`kagi`, `perplexity`, `valyu`, `sofya`, `firecrawl`; omitted when unset so NanoGPT routes), `outputType` (default
`searchResults`; also `sourcedAnswer`, `structured`).

`web_search` output: the resolved `query`, `provider`, `operation`, `depth`, `outputType`, timestamp, and USD cost
that the API returns in `metadata`, next to the raw `data` payload. Every one of those keys was checked against the
documented example response rather than guessed: the API names the cost `cost`, and the tool surface calls it
`costUsd`.

`web_scrape` input: `urls` (1 to 5), `stealthMode` (default `false`), `provider` (default `auto`; also `linkup`).

`web_scrape` output: `results[]` with `url`, `success`, `title`, `content`, `markdown`, and `error`, plus the raw
`summary`.

Tool handlers map the client's types onto their own, so the schema descriptions live in `internal/mcp` and the wire
types stay free of presentation concerns.

### URL cap

NanoGPT accepts at most 5 URLs per scrape, and the schema cannot express `maxItems`, so `web_scrape` rejects an empty
list or more than 5 before any request is sent.

### Cost and timeouts

- `stealthMode` multiplies the per-URL charge by 5 (0.0015 to 0.0075 per URL), so the tool description says so and
  `summary.stealthModeUsed` reports when it applied.
- The client timeout is 90s, below the server's write timeout, raised here from 60s to 120s. Scraping up to 5 pages
  through a stealth proxy can outlast a minute, and a client timeout that fires first returns a real tool error
  instead of a connection the server cuts mid-response. A test asserts the two timeouts stay ordered that way.

## Implementation units

### Unit 1: the NanoGPT client

Deliverables: `internal/nanogpt/{client,search,scrape}.go` with `{client,search,scrape}_test.go`.

Acceptance criteria:

- [x] Tests over `httptest` assert method, path, and that `web_search` sends `x-api-key` while `web_scrape` sends
      `Authorization: Bearer`, both against the configured base URL.
- [x] Tests assert the encoded search body carries `query` and the defaulted `depth` and `outputType`, and omits
      `provider` when unset; the scrape body carries `urls` and defaults `provider` to `auto`.
- [x] `stealthMode` is sent as `true` only when set, never as a defaulted field.
- [x] 401, 429, and 500 each produce an error naming the status and a body snippet; a 200 with malformed JSON
      produces a decode error; a 200 with a valid body decodes into the typed response.
- [x] `New` returns `ErrNoAPIKey` for an empty key.
- [x] No file in the package imports the MCP SDK or another internal package.
- [x] `go build ./...`, `go vet ./...`, and `go test ./... -race -count=1` pass.

### Unit 2: the tool surface and server wiring

Deliverables: `internal/mcp/{websearch,webscrape}.go` with tests, updated `internal/mcp/{server,handlers}.go`,
`internal/server/server.go` with its tests, `internal/config/config.go` with its test.

Acceptance criteria:

- [x] `tools/list` is exactly `["web_search","web_scrape"]` when a client is configured, and empty when it is not, so
      the bootstrap's empty-server case stays testable.
- [x] A session test drives both tools against an `httptest` NanoGPT and asserts structured output: search yields the
      resolved metadata and the payload, scrape yields one result per URL.
- [x] An upstream 500 reaches the client as a failed tool call whose text names the status, not as a successful
      result with an empty payload.
- [x] `web_scrape` rejects 0 and 6 URLs without issuing a request, and the error names the 1 to 5 range.
- [x] `server.New` returns `nanogpt.ErrNoAPIKey` for an empty `NANOGPT_API_KEY`, and starts otherwise.
- [x] The server's write timeout exceeds the client's, asserted by a test so a later change cannot invert them.
- [x] `/healthz`, `/mcp` auth, and the existing HTTP session tests pass, now over the two-tool surface.
- [x] `go build ./...`, `go vet ./...`, and `go test ./... -race -count=1` pass.

### Unit 3: documentation and the environment surface

Deliverables: `.env.example`, `README.md`, this plan.

Acceptance criteria:

- [x] `README.md` stays short and high level, and lists the two tools with their nano-gpt endpoints.
- [x] `.env.example` lists `NANOGPT_API_KEY` and `NANOGPT_BASE_URL` with their defaults, and no stealth-mode note,
      which belongs in the `web_scrape` description instead.
- [ ] Human check: with a real key, `web_search` returns results, `web_scrape` returns page text for a URL, and
      `stealthMode` on a scrape reports `stealthModeUsed` in the summary.

## Verification

Per unit: `go build ./...`, `go vet ./...`, `go test ./... -race -count=1`, and `gofmt -l .`. The acceptance criteria
that matter most are the exact tool list, the two different auth headers, and the non-2xx path, because those are the
three places a proxy can look correct while being wrong.

The one criterion left open needs a funded `NANOGPT_API_KEY` and a live endpoint, so it is a human check. Every other
criterion is covered by `httptest` stubs, which pin the wire format but cannot prove NanoGPT answers the way its
docs describe.
