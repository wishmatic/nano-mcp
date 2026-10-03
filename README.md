# nano-mcp

A minimal MCP server written in Go, bootstrapped from [go-mcp](https://github.com/wishmatic/go-mcp).

## Local Development

```sh
API_KEY=change-me go run ./cmd/server
```

The MCP endpoint is `/mcp`, and `/healthz` answers `ok` without a token.

```sh
docker run -d -p 8080:8080 -e API_KEY=change-me ghcr.io/wishmatic/nano-mcp:latest
```

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
