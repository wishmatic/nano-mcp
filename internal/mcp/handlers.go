package mcp

import (
	"github.com/wishmatic/nano-mcp/internal/filestore"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
	"github.com/wishmatic/nano-mcp/internal/resolve"
	"go.uber.org/zap"
)

type handlers struct {
	log      *zap.Logger
	nanogpt  *nanogpt.Client
	files    *filestore.Client
	resolver *resolve.Client
}

func (h *handlers) fail(tool string, err error) error {
	h.log.Warn(tool+" failed", zap.Error(err))

	return err
}
