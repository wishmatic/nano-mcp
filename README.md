# nano-mcp

An MCP server that exposes [nano-gpt](https://nano-gpt.com)'s data endpoints as tools, so a
client can search the web, read pages, and generate videos on nano-gpt's billing.

- `web_search` (`POST /api/web`): searches the web through nano-gpt's providers and
  reports the charge.
- `web_scrape` (`POST /api/scrape-urls`): reads up to 5 pages, at 5x cost in stealth mode.
- `firecrawl_scrape` (`POST /api/v1/firecrawl`): reads one page with Firecrawl's proxy and
  content filters.
- `firecrawl_map` (`POST /api/v1/firecrawl`): lists the URLs Firecrawl can discover on a site.
- `firecrawl_crawl` (`POST /api/v1/firecrawl`): crawls from a starting URL and returns the pages.
- `youtube_transcribe` (`POST /api/youtube-transcribe`): transcribes up to 10 YouTube videos, at
  about a cent each.
- `generate_video` (`POST /api/generate-video`): generates a video and waits it out, then serves
  a copy from this server as a resource link. MCP has no video content block, so the tool returns
  a link rather than media, and the server needs `PUBLIC_HOST` to build it.

The three firecrawl tools share one endpoint and differ by the `operation` they send.
`firecrawl_scrape` is worth having next to `web_scrape` because it is billed in Firecrawl credits
rather than per URL, and because it exposes a proxy tier and content filters `web_scrape` lacks.
`youtube_transcribe` bills only the videos that yield a transcript, so one without captions costs
nothing and comes back as a failed entry.

## Local Development

```sh
API_KEY=change-me NANOGPT_API_KEY=change-me PUBLIC_HOST=http://localhost:8080 go run ./cmd/server
```

The MCP endpoint is `/mcp`, and `/healthz` answers `ok` without a token. Generated videos are
served from `/v/`, under `FILES_DIR` on disk.

```sh
docker run -d -p 8080:8080 -v nano-mcp-files:/data \
  -e API_KEY=change-me -e NANOGPT_API_KEY=change-me \
  -e PUBLIC_HOST=https://nano-mcp.example.com \
  ghcr.io/wishmatic/nano-mcp:latest
```

`API_KEY` authenticates every `/mcp` request, `NANOGPT_API_KEY` funds every tool call, and
`PUBLIC_HOST` is where videos are served from; all three are required. A stored video is
readable by anyone who has its URL. See [.env.example](.env.example) for the rest.

## License

Licensed under the Apache License, Version 2.0, derived from
[go-mcp](https://github.com/wishmatic/go-mcp). See [LICENSE](LICENSE).
