package mcp

import (
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
	"go.uber.org/zap"
)

type handlers struct {
	log     *zap.Logger
	nanogpt *nanogpt.Client
}

func (h *handlers) fail(tool string, err error) error {
	h.log.Warn(tool+" failed", zap.Error(err))

	return err
}
