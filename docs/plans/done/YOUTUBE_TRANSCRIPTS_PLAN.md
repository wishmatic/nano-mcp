# YouTube transcripts

Status: Done
Depends on: `done/WEB_TOOLS_PLAN.md` and `done/FIRECRAWL_TOOLS_PLAN.md` (the `internal/nanogpt` client)

## Goal

Expose nano-gpt's YouTube transcription endpoint, `POST /api/youtube-transcribe`, as the MCP tool
`youtube_transcribe`, so an MCP client can pull the transcript of up to 10 videos per call.

## Non-goals

- Speech-to-text and its async job polling (`/api/v1/audio/transcriptions`, STT status). Those transcribe audio the
  caller already has, which is a different tool with a different billing model.
- Audio or video download, summarisation, translation, or diarisation.
- Retrying around the rate limit, or caching transcripts across calls.

## Design

### The endpoint, verified against the docs page

| Aspect     | Value                                                                           |
| ---------- | ------------------------------------------------------------------------------- |
| Path       | `/api/youtube-transcribe`                                                       |
| Auth       | `x-api-key: <key>`, the same header `web_search` uses                           |
| Body       | `{"urls": [...]}`, 1 to 10 URLs                                                 |
| Cost       | $0.01 per _successful_ transcript; failed URLs are not charged                  |
| Rate limit | 10 requests per minute per IP address                                           |
| Errors     | 400 for a bad array, 401 for a bad key, 402 for balance, 429 for the rate limit |

This page documents its response, unlike the Firecrawl adapter, so the output is modelled rather than passed
through:

```json
{
  "transcripts": [
    { "url": "...", "success": true, "title": "...", "transcript": "..." },
    { "url": "...", "success": false, "error": "Video not found or transcripts not available" }
  ],
  "summary": { "requested": 2, "processed": 2, "successful": 1, "failed": 1, "totalCost": 0.01 }
}
```

The `summary` is typed here, including `totalCost`, because all five of its fields are documented. `web_scrape`'s
summary stays `any` because only `stealthModeUsed` was documented for it.

### Client

One method, `TranscribeYouTube(ctx, urls) (*YouTubeTranscripts, error)`, over the existing `post` helper, sending
`x-api-key` and rejecting an empty list or more than `MaxTranscribeURLs` before any request, the way `Scrape` enforces
its own cap.

### Budget

Ten transcripts in one synchronous request can outlast the 90s `FastTimeout`, so this call gets its own budget:
`transcribeBase` (30s) plus `transcribePerURL` (15s) for each URL. At the 10-URL cap that is 180s, inside
`MaxCallBudget` (330s), and the server's write timeout (6m) already clears it. A single URL fails in 45s rather than
waiting out a ceiling sized for ten.

### Tool surface

`youtube_transcribe` takes `urls` (required) and returns the typed `transcripts` array plus the typed `summary`. The
description carries the two facts a caller cannot infer: it costs about a cent per video and only successful
transcripts are charged, and the endpoint allows 10 requests a minute.

## Implementation units

### Unit 1: the YouTube client

Deliverables: `internal/nanogpt/youtube.go` with `youtube_test.go`.

Acceptance criteria:

- [x] A test over `httptest` asserts `POST /api/youtube-transcribe`, the `x-api-key` header, no bearer header, and a
      body carrying exactly the requested URLs.
- [x] The response decodes into typed transcripts and a typed summary, including `totalCost` and a failed URL's
      `error`.
- [x] An empty list and 11 URLs are rejected without a request, naming the 1 to 10 range.
- [x] 400, 401, 402, and 429 each produce an error naming the status and quoting the body.
- [x] A malformed 200 produces a decode error.
- [x] The budget at `MaxTranscribeURLs` stays within `MaxCallBudget`.
- [x] `go build ./...`, `go vet ./...`, and `go test ./... -race -count=1` pass.

### Unit 2: the tool and its wiring

Deliverables: `internal/mcp/youtube.go` with `youtube_test.go`, updated `internal/mcp/server.go`,
`internal/mcp/server_test.go`, and `internal/server/http_test.go`.

Acceptance criteria:

- [x] `tools/list` is exactly the six tools, sorted, and still empty with no client configured.
- [x] A session test asserts the structured output: one entry per URL with its transcript, and a summary carrying
      the cost.
- [x] A failed URL comes back as a successful call with a failed entry, not as a tool error, since the API reports
      per-URL failures that way.
- [x] 11 URLs fail as a tool error naming the range, without a request.
- [x] An upstream 402 reaches the client as a failed tool call naming the balance problem.
- [x] The existing web and firecrawl tools' tests pass unchanged.
- [x] `go build ./...`, `go vet ./...`, and `go test ./... -race -count=1` pass.

### Unit 3: documentation

Deliverables: `README.md`, this plan.

Acceptance criteria:

- [x] `README.md` lists the tool with its endpoint and the per-success cost.
- [ ] Human check: with a real key, a real video URL returns its transcript, and a bogus video ID returns a failed
      entry rather than a tool error.

## Verification

Per unit: `go build ./...`, `go vet ./...`, `go test ./... -race -count=1`, and `gofmt -l .`. The criteria that
carry the most weight are the `x-api-key` header and the per-URL failure path, because a per-URL failure folded into
a tool error would hide which URL failed and why.
