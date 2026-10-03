# Video generation

Status: Done
Depends on: none
Related: `../elfu-mcp/docs/plans/done/FILES_AND_LATEST_PLAN.md`, `../neo-mcp`, `../elfu-mcp`

## Goal

Add one tool, `generate_video`, that submits to nano-gpt's `POST /api/generate-video`, waits the job out, and hands
back a URL to a copy this server hosts. MCP has no video content block, so a URL is the only way a generated video
can reach the caller; that is what forces a file server into a project that until now only read from nano-gpt.

nano-gpt's endpoint is asynchronous: submit returns a `runId` with `status: "pending"`, and the job is followed up on
a status endpoint. The tool hides that: a caller gets a finished video or an error, never a job id to poll, because
there is nowhere in a single tool call for that id to live between calls.

## Non-goals

- A second tool for the status endpoint. One tool, and it waits.
- `POST /api/generate-video/extend` and the faceswap, upscaler, and lipsync flows, which are separate shapes built on
  the same endpoint.
- nano-gpt's full request surface. It documents some sixty fields, most of them legacy pairs or single-model knobs;
  this adds the fields the documented models share, and waits for a model to need the rest.
- Sending media that is not already a public URL: no `imageDataUrl` or `videoDataUrl` base64 through tool arguments.
- Retention, eviction, and resumable or ranged downloads of stored files. A stored video is kept until the operator
  removes it, and is readable by anyone holding its URL, which matches `elfu-mcp` and `neo-mcp`.

## Design

### Submitting and waiting

`nanogpt.Client.GenerateVideo` submits, then polls `GET /api/video/status?requestId=<runId>` (the unified endpoint;
`/api/generate-video/status` is deprecated and needs `model` back, which buys nothing) until the job is terminal.

| Bound                        | Value | Why                                               |
| ---------------------------- | ----- | ------------------------------------------------- |
| `VideoPollInterval`          | 5s    | nano-gpt's own polling example uses 5s.           |
| `VideoWaitBudget`            | 10m   | The same example's whole window: 120 polls at 5s. |
| `filestore.MaxFetchDuration` | 3m    | Copying a completed video is a plain download.    |

`internal/server.writeTimeout` is `VideoWaitBudget + MaxFetchDuration + 1m`, so a call that stalls fails with the
upstream error rather than a connection the server cuts. The crawl budget is smaller, so nothing regresses.

A wait that runs out, a job that fails, and a job that is cancelled are errors, and every one of them names the
`runId`, so a caller who wants to follow up by hand has what it needs. A poll error that coincides with the budget
expiring reports the budget rather than `context deadline exceeded`, which would say nothing about the job.

### The two documented shapes

nano-gpt's unified status schema is flat (`status`, `videoUrl`, lowercase `completed`) while its examples are
wrapped (`data.status`, `data.output.video.url`, uppercase `COMPLETED`). The client reads both and normalises the
status through one function, including the `IN_QUEUE`/`IN_PROGRESS`/`CANCELED` spellings, because there is no way to
tell from here which one a given backend answers with.

### Hosting

`internal/filestore` mirrors `elfu-mcp/internal/filestore`: keys are `v/<YYYY-MM>/<uuid>.<ext>` under `FILES_DIR`,
served at `/v/*` with an immutable cache header, `nosniff`, and a media type derived from the extension. Only
`video/mp4`, `video/webm`, and `video/quicktime` are stored and served; anything else is refused rather than written,
so a mislabelled download cannot become a file with a type nothing declares.

Storing the copy rather than returning nano-gpt's URL is the point of the feature, not an optimisation:

- provider URLs expire, and the tool's contract is a link that keeps working;
- some providers need headers a plain client will not send;
- the public host is the one thing we know the caller can reach.

A provider that labels its output `application/octet-stream` is still stored, as `video/mp4`; the same video arrives
unlabelled often enough that refusing it would fail the common case to guard the rare one. `maxBytes` (nominally
256 MiB) caps a single download, and it is a field on the client so a test can drive the cap without writing a
quarter of a gigabyte.

The download is bounded by `MaxFetchDuration`, and the fetch is written atomically, so a caller can never open a
half-copied video and a failed download leaves nothing behind.

### Deployment

`PUBLIC_HOST` becomes required, because a generated video reaches the caller as a URL and only the operator knows
which host the caller can resolve. This matches `elfu-mcp` and `neo-mcp`; it is a config change for any existing
nano-mcp deployment, which fails at startup naming the variable rather than serving a tool that cannot work.

`FILES_DIR` defaults to `files`, and the image gains `WORKDIR /data` with a writable `/data`, so a volume can be
mounted where the siblings mount one.

### Interface

| Input            | Notes                                                                  |
| ---------------- | ---------------------------------------------------------------------- |
| `model`          | Required; video model id.                                              |
| `prompt`         | Required; every documented model wants one.                            |
| `negativePrompt` | `negative_prompt`.                                                     |
| `duration`       | String seconds, or `5s`, as models differ.                             |
| `aspectRatio`    | `aspect_ratio`.                                                        |
| `resolution`     | `resolution`.                                                          |
| `mode`           | `text-to-video`, `image-to-video`, `reference-to-video`, `video-edit`. |
| `seed`           | Pointer, so unset stays distinct from an explicit 0.                   |
| `generateAudio`  | Pointer, same reason.                                                  |
| `imageUrl`       | Source image for image-to-video.                                       |
| `videoUrl`       | Source video for extend and edit models.                               |

Output: `runId`, `model`, `videoUrl` (the hosted copy), `contentType`, `bytes`, `sourceUrl` (the provider URL the
copy came from, which may expire), `costUsd`.

The video itself is presented as a `ResourceLink` content block pointing at that URL, with its media type and size:
MCP has text, image, and audio content and no video content, so "referenced by URI" is the only shape a video can
take. The URL is repeated in the structured output, because a model reads structured content that a client may not
render as a link.

An unset optional field is omitted from the request body rather than sent empty, because nano-gpt warns that extra
media fields make some models fail validation.

## Implementation units

### Unit 1: configuration and the file store

Deliverables: `internal/config/config.go`, `internal/config/config_test.go`, `internal/filestore/store.go`,
`internal/filestore/paths.go`, `internal/filestore/handler.go`, `internal/filestore/{store,paths,handler}_test.go`.

Acceptance criteria:

- [x] `Config` gains `PUBLIC_HOST` and `FILES_DIR` (default `files`), and `PublicBase` accepts a trailing slash but
      rejects a missing scheme or host, a non-http scheme, a path, a query, or a fragment.
- [x] `PUBLIC_HOST` unset yields a nil base rather than an error, so the requirement is enforced in one place.
- [x] `Put` stores bytes under `v/<YYYY-MM>/<uuid>.<ext>`, returns the public URL, and the bytes read back off disk
      match; two puts never collide.
- [x] `Put` rejects a media type it does not serve, and honours a cancelled context.
- [x] `Fetch` copies the source, takes its media type from the response, and falls back to `video/mp4` when the
      source labels it `application/octet-stream`.
- [x] `Fetch` surfaces an upstream status, rejects a non-http source and a source that is not a URL, and refuses an
      asset over the byte cap.
- [x] `safePath` rejects empty, absolute, out-of-namespace, `.`, `..`, and empty-segment keys, and keeps a valid key
      under the storage directory.
- [x] The routes serve the stored bytes with the declared media type, an immutable cache header, and `nosniff`;
      they answer `HEAD` with no body; and they 404 an unknown extension, an unstored key, and a traversing path.

### Unit 2: the video client

Deliverables: `internal/nanogpt/client.go`, `internal/nanogpt/video.go`, `internal/nanogpt/video_test.go`.

Acceptance criteria:

- [x] `Client` gains a `get` alongside `post`, both sharing one request path, with the payload marshalled only when
      there is one.
- [x] Submitting sends `x-api-key`, `model`, `prompt`, and each optional field mapped to nano-gpt's name
      (`negative_prompt`, `aspect_ratio`, `imageUrl`, ...), and omits every field the caller did not set.
- [x] The tool polls until the job is terminal: `IN_QUEUE` and `IN_PROGRESS` are not terminal, and the number of
      polls matches the number of non-terminal statuses plus the completed one.
- [x] Both documented status shapes are read: nested `data.status` with `data.output.video.url`, and flat `status`
      with `videoUrl`.
- [x] The cost nano-gpt reports on completion is returned.
- [x] `FAILED` and `CANCELED` are errors carrying nano-gpt's reason, preferring the raw `error` over
      `userFriendlyError`, and a `COMPLETED` job with no URL is an error too.
- [x] Every failure names the `runId`.
- [x] A job still running when the budget expires reports the last status and the run id, not
      `context deadline exceeded`.
- [x] A poll that fails is surfaced with its status code and message, and a submit response without a `runId` is
      rejected.
- [x] An empty or blank `model` or `prompt` is rejected before any upstream call.

### Unit 3: the tool

Deliverables: `internal/mcp/video.go`, `internal/mcp/video_test.go`, `internal/mcp/server.go`,
`internal/mcp/handlers.go`, `internal/mcp/testhelpers_test.go`.

Acceptance criteria:

- [x] `mcp.Deps` gains `Files`, and `generate_video` is registered only when both a nano-gpt client and a file store
      are configured, so a partially built server lists only tools that work.
- [x] The input schema carries `model`, `prompt`, `negativePrompt`, `duration`, `aspectRatio`, `resolution`, `seed`,
      `generateAudio`, `imageUrl`, and `videoUrl`.
- [x] A successful call returns the run id, model, hosted URL, media type, byte count, the provider URL, and the cost;
      the hosted file on disk holds the provider's bytes.
- [x] The result carries one `ResourceLink` to the hosted copy, with its media type and size, and no media block.
- [x] A missing `model` or `prompt` is an error naming the field.
- [x] An upstream refusal is an error carrying the status and nano-gpt's message.
- [x] `internal/mcp` still lists the other six tools when no file store is configured.

### Unit 4: wiring and deployment

Deliverables: `internal/server/server.go`, `internal/server/server_test.go`, `internal/server/http_test.go`,
`Dockerfile`, `.env.example`, `README.md`.

Acceptance criteria:

- [x] `newServer` refuses to start without `PUBLIC_HOST`, naming it, and rejects every `PUBLIC_HOST` shape `PublicBase`
      rejects.
- [x] The server mounts the file routes and logs that stored videos are publicly readable.
- [x] `writeTimeout` clears both the crawl budget and a video call's wait plus download, asserted against the
      constants rather than a literal.
- [x] An MCP client over real HTTP lists `generate_video` alongside the other tools.
- [x] The Dockerfile gains a writable `/data` and `WORKDIR /data`, the way `elfu-mcp` and `neo-mcp`
      do, and the README and `.env.example` document `PUBLIC_HOST` and `FILES_DIR`.
- [x] `go test -race ./...` passes.

### Unit 5: human verification

- [ ] Human check: `docker build` succeeds and a container started with `API_KEY`, `NANOGPT_API_KEY`, and
      `PUBLIC_HOST` serves a video from `/v/` under a mounted `/data` volume; the local Docker socket was not
      reachable here, so only the Dockerfile's text was checked.
- [ ] Human check: with a funded `NANOGPT_API_KEY` and a reachable `PUBLIC_HOST`, call `generate_video` from a real
      MCP client with a short model (`kling_v2_1_std_5s`, `duration: "5"`) and confirm the returned URL plays the
      video in a browser.
- [ ] Human check: confirm a model that a prompt violates returns nano-gpt's content-policy message through the tool
      rather than a generic failure.
- [ ] Human check: confirm the stored copy lands under `FILES_DIR` on the mounted volume, and that a volume
      remount keeps earlier videos reachable.
