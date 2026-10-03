# Input address resolution

Status: Done
Depends on: none
Related: `docs/plans/done/VIDEO_GENERATION_PLAN.md`, `../elfu-mcp`, `../neo-mcp`

## Goal

Let `generate_video` take an _address_ for its input image instead of only the address of something nano-gpt can
fetch itself: a URL, a data URI, raw base64, or a URL this deployment maps onto a file on disk. That is what makes
image-to-video work at all, because the image a caller wants animated is usually one this deployment can read and
nano-gpt cannot: an `elfu-mcp` or `neo-mcp` file, or an upload behind a chat front end.

Then `image` on the tool means "animate this", and the call switches to `image-to-video` unless the caller named
a mode.

## Non-goals

- Video-to-video. `videoUrl` stays a passthrough for an address the caller already knows nano-gpt can fetch; the
  same resolution treatment for a source video comes with the extend flows.
- Reading arbitrary host paths. A file on disk is reachable only through `IMAGE_URL_MAP`, which is the point of the
  map: what is readable is what the operator mapped, and the public side of an entry is that entry's access control.
- Re-encoding, downscaling, or transcoding an input. The bytes travel as they were found.
- Uploading to nano-gpt's attachment library (`imageAttachmentId`). There is no documented endpoint for it, and the
  inline form needs no round trip.

## Design

### Resolve, do not pass the address on

nano-gpt takes `imageUrl` or `imageDataUrl`, and its docs recommend the inline form for private assets. Resolving to
bytes and sending `imageDataUrl` is the only option that holds in every deployment:

- an `elfu-mcp` address is usually a host only this deployment can reach, so passing it on fails;
- an address this deployment reads off disk has no URL at all;
- a `PUBLIC_HOST` that is LAN-only would leave an `imageUrl` unreadable by nano-gpt, and the resulting failure
  would arrive as an opaque job error.

The cost is that the request body carries the image base64-encoded, a third larger than the file, so
`resolve.MaxImageBytes` (16 MiB) caps what will be inlined. Beyond that the caller is told the size and the cap.

### The three packages, and where they came from

`internal/utils`, `internal/sourcemap`, and `internal/resolve` are `elfu-mcp`'s and `neo-mcp`'s, which are the same
file modulo the module name. Two deliberate differences:

- `resolve.New` takes only the address map. The siblings' object-store and public-base branches exist so a URL this
  service itself served can be read from disk instead of over the network; nothing nano-mcp can animate is served by
  nano-mcp, so that plumbing would be dead code. The map already covers the case: a directory entry pointing at
  `FILES_DIR` reads a stored file locally.
- `Resolve` enforces `MaxImageBytes`, because the ceiling that matters here is the request body, not the encode.

An address is read as, in order: a `data:` URI, an http(s) URL (rewritten through the map on every redirect hop),
or base64. Addresses are never stat'ed, so there is no way to name a path except through the map.

A mapped _directory_ entry reads the file directly. A mapped _base URL_ entry rewrites the address and fetches it,
which covers a public host that is only reachable under a private name. An unmapped address is fetched as given.

### What reaches nano-gpt

```json
{
  "model": "kling_v2_1_std_5s",
  "prompt": "...",
  "mode": "image-to-video",
  "imageDataUrl": "data:image/png;base64,..."
}
```

`mode` is filled in only when the caller left it empty, so `reference-to-video` or a model-specific mode survives.
`imageDataUrl` is built from the resolved media type, which is detected from the bytes with the address's declared
type as the fallback, and limited to `image/png`, `image/jpeg`, and `image/webp` the way the siblings limit theirs.

## Implementation units

### Unit 1: the address map

Deliverables: `internal/utils/http.go`, `internal/utils/io.go`, `internal/sourcemap/sourcemap.go` and their tests.

Acceptance criteria:

- [x] `utils.IsHTTP` accepts http and https URLs only, and `utils.ReadLimited` trims and truncates.
- [x] `sourcemap.Parse` reads comma-separated `public=private` pairs, where the private side is an absolute
      directory or an absolute http(s) URL, and rejects a pair with no `=`, a duplicate public side, a public side
      that is not http(s), and a private side that is neither.
- [x] `Lookup` picks the longest matching public path prefix, matches scheme and host case-insensitively, and
      returns nothing for a host it does not know.
- [x] A directory source reads a file under its directory and refuses an absolute path, a traversal, an extension it
      does not read, a file over `maxFileBytes`, and a path that is not a regular file.
- [x] A base URL source rewrites the address onto its base, keeping the path remainder and the query.

### Unit 2: the resolver

Deliverables: `internal/resolve/resolver.go`, `internal/resolve/input.go` and their tests.

Acceptance criteria:

- [x] `Resolve` reads a png, jpeg, or webp from a data URI, from raw base64 in any of the four base64 alphabets,
      with embedded whitespace, and from a URL.
- [x] The media type comes from the bytes when they say something it serves, then from the declared type, and is an
      error when neither is a type it serves.
- [x] An empty address, a malformed data URI, a data URI that is not base64, and invalid base64 are errors, each
      prefixed with `resolve:`.
- [x] An image over `MaxImageBytes` is an error naming the size and the cap.
- [x] `Fetch` follows redirects itself, gives up after `maxRedirects` with the limit named, rejects a redirect with
      no `Location`, surfaces a non-200, and sends a descriptive User-Agent on every hop.
- [x] A mapped host is read from its directory, a mapped base URL is fetched from its target, and an unmapped
      address is fetched as given.
- [x] `sourcemap.Parse`'s spec is honoured through the resolver, so a mapped URL that escapes its directory, names
      an extension the map does not read, or does not exist is an error.

### Unit 3: the tool

Deliverables: `internal/mcp/video.go`, `internal/mcp/server.go`, `internal/mcp/handlers.go`,
`internal/mcp/video_test.go`, `internal/mcp/testhelpers_test.go`.

Acceptance criteria:

- [x] `generate_video`'s input carries `image` in place of `imageUrl`, with the accepted address forms in its
      schema, and `videoUrl` documented as a passthrough.
- [x] An `image` that resolves is sent as `imageDataUrl`, with the resolved media type and the resolved bytes.
- [x] An `image` switches `mode` to `image-to-video` when the caller left it empty, and leaves a caller's own mode
      alone.
- [x] An image that cannot be resolved fails the call before anything is submitted, with the resolver's reason.
- [x] A call with no `image` is unchanged: no `imageDataUrl` and no `mode` in the request.
- [x] `mcp.Deps` gains `Resolver`, and `generate_video` is registered only when the client, the file store, and the
      resolver are all configured.

### Unit 4: wiring and deployment

Deliverables: `internal/config/config.go` and its test, `internal/server/server.go` and its test, `.env.example`,
`README.md`.

Acceptance criteria:

- [x] `IMAGE_URL_MAP` is read into the config, and a malformed map fails startup naming the variable.
- [x] The server builds the address map and the resolver, and passes the resolver to the MCP server.
- [x] `.env.example` documents the pair syntax, a chat front end's uploads, a sibling MCP server's files, and a
      private base URL, with the note that a directory entry's public side is its access control.
- [x] The README explains that the image is read here and travels with the request, and shows a map entry.
- [x] `go build ./...`, `go vet ./...`, and `go test -race ./...` pass.

### Unit 5: human verification

- [ ] Human check: with `IMAGE_URL_MAP` pointing at a mounted images directory, call `generate_video` with `image`
      naming the public side of a real file and confirm the finished video resembles it.
- [ ] Human check: confirm that a map entry's public side is the only thing needed, with the private directory not
      reachable over the network.
- [ ] Human check: generate a video from an image at `elfu-mcp`'s public host with its files directory mounted, and
      confirm no request left the host for that image.
