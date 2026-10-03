package resolve

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/wishmatic/nano-mcp/internal/utils"
)

const (
	MediaPNG  = "image/png"
	MediaJPEG = "image/jpeg"
	MediaWebP = "image/webp"

	// MaxImageBytes caps what travels to nano-gpt, where the image is base64-encoded into the
	// request body and so costs a third more on the wire than it does on disk.
	MaxImageBytes = 16 << 20
)

type Image struct {
	Data      []byte
	MediaType string
}

// Resolve normalises an image reference into raw bytes and a media type. The reference may be an http(s) URL, a
// base64 data URI, or raw base64 data; URLs go through Fetch, so redirects and mapped hosts are shared.
func (c *Client) Resolve(ctx context.Context, input string) (Image, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return Image{}, fmt.Errorf("resolve: empty image input")
	}

	var (
		data         []byte
		declaredType string
		err          error
	)

	switch {
	case strings.HasPrefix(trimmed, "data:"):
		data, declaredType, err = decodeDataURI(trimmed)
	case utils.IsHTTP(trimmed):
		data, err = c.Fetch(ctx, trimmed)
	default:
		data, err = decodeBase64(trimmed)
	}
	if err != nil {
		return Image{}, err
	}

	if int64(len(data)) > c.maxBytes {
		return Image{}, fmt.Errorf("resolve: the image is %d bytes, over the %d byte limit", len(data), c.maxBytes)
	}

	mediaType, err := supportedMediaType(data, declaredType)
	if err != nil {
		return Image{}, err
	}

	return Image{Data: data, MediaType: mediaType}, nil
}

func decodeDataURI(uri string) ([]byte, string, error) {
	meta, payload, ok := strings.Cut(uri[len("data:"):], ",")
	if !ok {
		return nil, "", fmt.Errorf("resolve: malformed data URI: missing comma")
	}

	if !strings.HasSuffix(strings.ToLower(meta), ";base64") {
		return nil, "", fmt.Errorf("resolve: unsupported data URI: only base64-encoded data is supported")
	}

	declared := meta[:len(meta)-len(";base64")]
	if i := strings.Index(declared, ";"); i >= 0 {
		declared = declared[:i]
	}

	data, err := decodeBase64(payload)
	if err != nil {
		return nil, "", err
	}

	return data, strings.ToLower(strings.TrimSpace(declared)), nil
}

func decodeBase64(encoded string) ([]byte, error) {
	cleaned := strings.Join(strings.Fields(encoded), "")
	if cleaned == "" {
		return nil, fmt.Errorf("resolve: empty base64 data")
	}

	for _, encoding := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		data, err := encoding.DecodeString(cleaned)
		if err == nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("resolve: invalid base64 image data")
}

func supportedMediaType(data []byte, declared string) (string, error) {
	detected := http.DetectContentType(data)

	switch {
	case isSupportedMediaType(detected):
		return detected, nil
	case isSupportedMediaType(declared):
		return declared, nil
	case declared != "":
		return "", fmt.Errorf("resolve: unsupported image media type %q", declared)
	default:
		return "", fmt.Errorf("resolve: unsupported image media type %q", detected)
	}
}

func isSupportedMediaType(mediaType string) bool {
	switch mediaType {
	case MediaPNG, MediaJPEG, MediaWebP:
		return true
	default:
		return false
	}
}
