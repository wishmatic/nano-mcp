package utils

import (
	"io"
	"strings"
)

const maxReadSize = 4 * 1024

func ReadLimited(r io.Reader) string {
	if r == nil {
		return ""
	}

	b, _ := io.ReadAll(io.LimitReader(r, maxReadSize))

	return strings.TrimSpace(string(b))
}
