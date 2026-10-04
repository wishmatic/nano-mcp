package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/filestore"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
	"github.com/wishmatic/nano-mcp/internal/resolve"
	"go.uber.org/zap"
)

type Deps struct {
	Log      *zap.Logger
	NanoGPT  *nanogpt.Client
	Files    *filestore.Client
	Resolver *resolve.Client
}

const version = "0.1.0"

func New(deps Deps) (*mcp.Server, error) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "nano-mcp",
		Version: version,
	}, nil)

	registerTools(srv, &handlers{
		log:      deps.Log,
		nanogpt:  deps.NanoGPT,
		files:    deps.Files,
		resolver: deps.Resolver,
	})

	return srv, nil
}

// A server built without a nano-gpt client lists no tools, which keeps the
// empty surface reachable in tests and in any future keyless deployment.
func registerTools(srv *mcp.Server, h *handlers) {
	if h.nanogpt == nil {
		return
	}

	registerWebSearch(srv, h)
	registerWebScrape(srv, h)
	registerFirecrawlScrape(srv, h)
	registerFirecrawlMap(srv, h)
	registerFirecrawlCrawl(srv, h)
	registerYouTubeTranscribe(srv, h)
	registerListModels(srv, h)
	registerCheckBalance(srv, h)

	// generate_video and generate_image are the tools that need somewhere to put what they make,
	// and something that can turn the address they are handed into the bytes they upload.
	if h.files != nil && h.resolver != nil {
		registerGenerateVideo(srv, h)
		registerGenerateImage(srv, h)
	}
}
