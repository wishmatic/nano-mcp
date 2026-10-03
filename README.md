# nano-mcp

An MCP server for [nano-gpt](https://nano-gpt.com)'s data endpoints, so an MCP client can search the web and read
pages on nano-gpt's billing.

| Tool               | nano-gpt endpoint        | What it does                                                                        |
| ------------------ | ------------------------ | ----------------------------------------------------------------------------------- |
| `web_search`       | `POST /api/web`          | Searches the web through one of nano-gpt's providers, reporting what it charged.    |
| `web_scrape`       | `POST /api/scrape-urls`  | Fetches the readable text of up to 5 pages, at 5x the per-URL cost in stealth mode. |
| `firecrawl_scrape` | `POST /api/v1/firecrawl` | Fetches one page with Firecrawl's proxy and content controls.                       |
| `firecrawl_map`    | `POST /api/v1/firecrawl` | Lists the URLs Firecrawl can discover on a site.                                    |
| `firecrawl_crawl`  | `POST /api/v1/firecrawl` | Crawls a site from a starting URL and returns the scraped pages.                    |

The three firecrawl tools share one endpoint and differ by the `operation` they send. `firecrawl_scrape` is worth
having next to `web_scrape` because it is billed in Firecrawl credits rather than per URL, and because it exposes a
proxy tier and content filters that `web_scrape` has no equivalent for.

## Local Development

```sh
API_KEY=change-me NANOGPT_API_KEY=change-me go run ./cmd/server
```

The MCP endpoint is `/mcp`, and `/healthz` answers `ok` without a token.

```sh
docker run -d -p 8080:8080 -e API_KEY=change-me -e NANOGPT_API_KEY=change-me ghcr.io/wishmatic/nano-mcp:latest
```

`API_KEY` authenticates every `/mcp` request and `NANOGPT_API_KEY` funds every tool call; both are required. See
[.env.example](.env.example) for the rest.

## License

Licensed under the Apache License, Version 2.0, derived from [go-mcp](https://github.com/wishmatic/go-mcp). See
[LICENSE](LICENSE).
