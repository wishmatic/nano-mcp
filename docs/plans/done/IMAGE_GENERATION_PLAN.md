# Image generation

Status: Done
Depends on: `docs/plans/done/VIDEO_GENERATION_PLAN.md`, `docs/plans/done/INPUT_RESOLUTION_PLAN.md`
Related: `../neo-mcp/internal/present`, `../neo-mcp/internal/mcp/publish.go`

## Goal

Add one tool, `generate_image`, that calls nano-gpt's OpenAI-compatible
`POST /api/v1/images/generations` and hands back the images themselves.

Unlike video, MCP has an image content block, so a generated image reaches the caller as an attachment
rather than as a link: the result carries the URL this server serves its copy from as text, immediately
followed by the image. That pairing is `neo-mcp`'s presentation contract, and the reason this tool needs
`PUBLIC_HOST` and the file store at all is only that the URL is worth keeping.

The call is synchronous; there is no job to poll, which is what keeps this half the size of
`generate_video`.

## Non-goals

- The dedicated `POST /api/v1/images` endpoint and `GET /api/v1/image-models` discovery. The
  OpenAI-compatible endpoint is one shape for every model, which is what makes the client portable to
  `neo-mcp`.
- `POST /api/v1/images/edits`, which is a separate shape for the same models.
- Shrinking the copy attached to the result. `neo-mcp` has `internal/format` and `internal/present` for
  that; here the bytes nano-gpt returned are attached as they are, and an operator who wants a budget
  should lift those packages rather than have this tool grow a half-sized one.
- `kontext_max_mode`, a single-model flag. The rest of the documented knobs are exposed.
- `user`, which identifies an end user to nano-gpt and has no meaning for a tool call.

## Design

### The endpoint

`nanogpt.Client.GenerateImages` posts to `/api/v1/images/generations` with `Authorization: Bearer`,
the header the other OpenAI-compatible endpoint (`/api/v1/firecrawl`) uses rather than the `x-api-key`
the video endpoint wants.

| Bound                        | Value | Why                                                                |
| ---------------------------- | ----- | ------------------------------------------------------------------ |
| `ImageGenerationBudget`      | 5m    | Synchronous generation holds the connection; `FastTimeout` is 90s. |
| `filestore.MaxFetchDuration` | 3m    | Copying an image the API returned as a URL is a plain download.    |

`internal/server.writeTimeout` is already `VideoWaitBudget + MaxFetchDuration + 1m`, which clears an
image call's `ImageGenerationBudget + MaxFetchDuration`; the video call stays the longest, so the
constant does not move, but the assertion in `server_test.go` gains the image bound so that a later
change to either cannot silently outgrow it.

### Base64 first, URL as the fallback

nano-gpt documents `response_format: "b64_json"` as the default, and warns that a request for `"url"`
may still come back as `b64_json` when its upload fails. So the tool asks for `b64_json` outright and
the client reads whichever of the two each `data[i]` carries, never both:

- bytes inline: the media type is sniffed from the bytes, because nano-gpt names none;
- a signed URL: `filestore.Fetch` copies it, which is also where the `Content-Type` is read.

Asking for bytes is what makes the client portable: `neo-mcp`'s store publishes bytes too, and neither
has to reach for a URL that expires within the hour.

### Storing and presenting

`filestore` grows `image/png`, `image/jpeg`, and `image/webp` alongside the three video types, so a
generated image is served from the same `/v/*` routes under the same namespace; the namespace is left
alone, since renaming it would strand what is already on disk.

`Fetch` also returns the bytes it stored, because the image has to be attached to the result as well as
written to disk, and re-reading the file or re-downloading the URL to get them is absurd. It also
sniffs the downloaded bytes when the source labels them `application/octet-stream`, which is what an
image off a mislabelling provider arrives as; the `video/mp4` default is unchanged for bytes that
sniff as nothing this server serves.

Presentation, following `neo-mcp`'s `present.StoredImages`:

| Content block | Value                                                              |
| ------------- | ------------------------------------------------------------------ |
| text          | the URL the copy is served from, one per image                     |
| image         | the bytes, with the stored media type, audience user and assistant |

The audience is what lets a vision-capable model see the image rather than only the URL. The URL is
also in the structured output, because a client may render the image block and drop the text.

### Interface

| Input               | Request field         | Notes                                                         |
| ------------------- | --------------------- | ------------------------------------------------------------- |
| `prompt`            | `prompt`              | Required.                                                     |
| `model`             | `model`               | Optional; unset is nano-gpt's documented `hidream` default.   |
| `n`                 | `n`                   | Pointer, so unset is distinct from an explicit count.         |
| `size`              | `size`                | `1024x1024`, `1376x768`, `auto`; model-specific.              |
| `image`             | `imageDataUrl`        | Resolved through `resolve`; switches the call to img2img.     |
| `images`            | `imageDataUrls`       | The same, for the models that take several.                   |
| `mask`              | `maskDataUrl`         | Resolved the same way; white marks what to generate.          |
| `seed`              | `seed`                | Pointer; may improve reproducibility without guaranteeing it. |
| `strength`          | `strength`            | Pointer; 0 to 1.                                              |
| `guidanceScale`     | `guidance_scale`      | Pointer; 0 to 20.                                             |
| `numInferenceSteps` | `num_inference_steps` | Pointer; 1 to 100.                                            |

Output: `model` (what was asked for, empty meaning nano-gpt's default), `count`, `urls`, `costUsd`.

Every address the caller names is read here and travels as a data URL, matching `generate_video`'s
`image`: nano-gpt documents that it takes data URLs and not addresses, and the image a caller wants
transformed is usually one only this deployment can reach.

An unset optional field is omitted from the request body rather than sent empty, because nano-gpt
validates that a model does not receive fields it has no use for.

## Implementation units

### Unit 1: the image client

Deliverables: `internal/nanogpt/images.go`, `internal/nanogpt/images_test.go`.

Acceptance criteria:

- [x] `GenerateImages` posts to `/api/v1/images/generations` with `Authorization: Bearer`, and every
      exposed field is sent under nano-gpt's own name.
- [x] `b64_json` is decoded into bytes, and a `data:image/...;base64,` prefix or missing padding does not
      defeat the decode.
- [x] An entry carrying a `url` instead of `b64_json` is returned as that URL, so the fallback the docs
      warn about is handled rather than dropped.
- [x] An entry carrying neither, and a response with no entries at all, are errors naming which image.
- [x] The `cost` nano-gpt reports is returned.
- [x] An empty or blank `prompt` is rejected before any upstream call, and an upstream refusal is
      surfaced with its status and body.
- [x] Unset optional fields are omitted from the request body.

### Unit 2: images in the file store

Deliverables: `internal/filestore/store.go`, `internal/filestore/handler.go`,
`internal/filestore/{store,handler}_test.go`.

Acceptance criteria:

- [x] `image/png`, `image/jpeg`, and `image/webp` are stored, served with the right `Content-Type`, and
      round-trip their bytes; a type that is still not served is still refused by `Put` and 404s on the
      route.
- [x] `DetectMediaType` names each type Go can sniff out of the six this server serves (png, jpeg, webp,
      mp4, webm; QuickTime has no signature) and reports false for bytes that are none of them.
- [x] `Fetch` returns the stored bytes as well as the asset, and they match what landed on disk.
- [x] `Fetch` takes an image type from a source that labels it `image/png`, and sniffs one out of a
      source that labels it `application/octet-stream`.
- [x] The `video/mp4` default still applies to bytes that are neither labelled nor recognisable.

### Unit 3: the tool

Deliverables: `internal/mcp/image.go`, `internal/mcp/image_test.go`, `internal/mcp/server.go`.

Acceptance criteria:

- [x] The input schema carries `prompt`, `model`, `n`, `size`, `image`, `images`, `mask`, `seed`,
      `strength`, `guidanceScale`, and `numInferenceSteps`.
- [x] A successful call returns `count`, the hosted `urls`, the model, and the cost, and the files on
      disk hold the bytes nano-gpt returned.
- [x] The result is `[text(url), image, ...]` per image: the URL immediately before the image, and no
      resource link.
- [x] The attached image carries the stored media type, the bytes, and the user-and-assistant audience.
- [x] An image the API returned as a URL is downloaded, stored, and attached.
- [x] `image`, `images`, and `mask` are resolved here and sent as data URLs, and a failure to resolve one
      fails the call before anything is submitted.
- [x] A missing `prompt` is an error naming the field, and an upstream refusal is an error carrying the
      status and nano-gpt's message.
- [x] `generate_image` is registered only when the client, the file store, and the resolver are all
      configured, and `internal/mcp` still lists the other tools without them.

### Unit 4: wiring and documentation

Deliverables: `internal/server/server.go`, `internal/server/server_test.go`,
`internal/server/http_test.go`, `internal/mcp/server_test.go`, `README.md`, `.env.example`.

Acceptance criteria:

- [x] `writeTimeout` clears an image call's budget plus its download, asserted against the constants
      rather than a literal.
- [x] The startup log says media is hosted rather than only videos, and still warns that anything stored
      is readable by whoever holds its URL.
- [x] A real MCP client over HTTP lists `generate_image` alongside the other tools.
- [x] The README documents the tool, that images are attached as well as linked, and that `PUBLIC_HOST`
      and `FILES_DIR` cover images too; `.env.example` says the address map feeds both tools.

### Unit 5: human verification

- [ ] Human check: with a funded `NANOGPT_API_KEY` and a reachable `PUBLIC_HOST`, call `generate_image`
      from a real MCP client and confirm the image renders in the result and the URL it names serves the
      same bytes.
- [ ] Human check: pass `image` naming a file in a mapped directory and confirm the generated image
      resembles it, and that no request left the host for that file.
- [ ] Human check: confirm a `n` above 1 returns that many images, each with its own URL, and that the
      reported `costUsd` matches nano-gpt's own record.
