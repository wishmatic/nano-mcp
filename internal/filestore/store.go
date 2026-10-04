// Package filestore keeps the media nano-gpt generates on disk and serves it over HTTP. A
// generated video reaches the caller as a URL to one of these files rather than as tool content,
// because MCP has no video content block; a generated image is stored the same way, and is
// attached to the result as well.
package filestore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	namespace = "v"
	dirMode   = 0o750
	fileMode  = 0o640

	// MaxAssetBytes caps a single download, so a runaway upstream or an endless stream cannot
	// fill the disk.
	MaxAssetBytes = 256 << 20

	// MaxFetchDuration bounds one download; internal/server's write timeout clears it.
	MaxFetchDuration = 3 * time.Minute
)

type Config struct {
	Dir        string
	PublicBase *url.URL
}

type Client struct {
	cfg  Config
	log  *zap.Logger
	http *http.Client

	// maxBytes is MaxAssetBytes, and is a field so a test can drive the cap without writing
	// a quarter of a gigabyte.
	maxBytes int64
}

type Asset struct {
	URL       string
	MediaType string
	Bytes     int64
}

func New(cfg Config, log *zap.Logger) (*Client, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("filestore: a storage directory is required")
	}

	if cfg.PublicBase == nil || cfg.PublicBase.Host == "" {
		return nil, fmt.Errorf("filestore: a public base URL is required")
	}

	if err := os.MkdirAll(cfg.Dir, dirMode); err != nil {
		return nil, fmt.Errorf("filestore: create %s: %w", cfg.Dir, err)
	}

	return &Client{cfg: cfg, log: log, http: &http.Client{}, maxBytes: MaxAssetBytes}, nil
}

func (c *Client) Put(ctx context.Context, data []byte, mediaType string) (Asset, error) {
	if err := ctx.Err(); err != nil {
		return Asset{}, err
	}

	ext, ok := extensionFor(mediaType)
	if !ok {
		return Asset{}, fmt.Errorf("filestore: cannot store %q", mediaType)
	}

	key := objectKey(ext)

	path, err := c.safePath(key)
	if err != nil {
		return Asset{}, err
	}

	if err := writeFileAtomic(path, data); err != nil {
		return Asset{}, fmt.Errorf("filestore: store %s: %w", key, err)
	}

	c.log.Info("stored asset",
		zap.String("key", key),
		zap.String("content_type", mediaType),
		zap.Int("bytes", len(data)),
	)

	return Asset{URL: c.url(key), MediaType: mediaType, Bytes: int64(len(data))}, nil
}

// Fetch copies an upstream asset to disk. nano-gpt hands back a provider URL that expires, and
// some providers require headers a plain reader will not send, so a caller gets a link of our
// own instead. The bytes come back too, because a caller presenting the asset inline needs them
// and re-reading the file would be absurd.
func (c *Client) Fetch(ctx context.Context, source string) (Asset, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, MaxFetchDuration)
	defer cancel()

	if err := checkSource(source); err != nil {
		return Asset{}, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return Asset{}, nil, fmt.Errorf("filestore: build request for %s: %w", source, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Asset{}, nil, fmt.Errorf("filestore: download %s: %w", source, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Asset{}, nil, fmt.Errorf("filestore: download %s: %s", source, resp.Status)
	}

	data, err := c.readCapped(resp.Body)
	if err != nil {
		return Asset{}, nil, fmt.Errorf("filestore: download %s: %w", source, err)
	}

	asset, err := c.Put(ctx, data, mediaTypeOf(resp.Header.Get("Content-Type"), data))
	if err != nil {
		return Asset{}, nil, err
	}

	return asset, data, nil
}

func checkSource(source string) error {
	parsed, err := url.Parse(source)
	if err != nil {
		return fmt.Errorf("filestore: %q is not a URL: %w", source, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("filestore: %q is not an http URL", source)
	}

	return nil
}

func (c *Client) readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, c.maxBytes+1))
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > c.maxBytes {
		return nil, fmt.Errorf("the asset exceeds the %d byte cap", c.maxBytes)
	}

	return data, nil
}

func (c *Client) url(key string) string {
	base := *c.cfg.PublicBase
	base.Path = strings.TrimSuffix(base.Path, "/") + "/" + key

	return base.String()
}

func objectKey(ext string) string {
	parts := []string{namespace, time.Now().UTC().Format("2006-01"), uuid.NewString() + "." + ext}

	return strings.Join(parts, "/")
}
