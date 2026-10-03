package filestore

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

const defaultMediaType = "video/mp4"

var mediaTypes = map[string]string{
	"video/mp4":       "mp4",
	"video/webm":      "webm",
	"video/quicktime": "mov",
}

func (c *Client) Register(r chi.Router) {
	r.Get("/"+namespace+"/*", c.serve)
	r.Head("/"+namespace+"/*", c.serve)
}

func (c *Client) serve(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/")

	mediaType, ok := mediaTypeFor(key)
	if !ok {
		http.NotFound(w, r)

		return
	}

	path, err := c.safePath(key)
	if err != nil {
		http.NotFound(w, r)

		return
	}

	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)

		return
	}

	defer file.Close()

	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)

		return
	}

	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

func mediaTypeFor(key string) (string, bool) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(key), "."))

	for mediaType, candidate := range mediaTypes {
		if candidate == ext {
			return mediaType, true
		}
	}

	return "", false
}

func extensionFor(mediaType string) (string, bool) {
	ext, ok := mediaTypes[strings.ToLower(strings.TrimSpace(mediaType))]

	return ext, ok
}

// mediaTypeOf trusts a provider's content type only when it names a format we serve, because
// the same assets arrive labelled application/octet-stream or with no type at all.
func mediaTypeOf(header string) string {
	base, _, _ := strings.Cut(header, ";")

	if _, ok := mediaTypes[strings.ToLower(strings.TrimSpace(base))]; ok {
		return strings.ToLower(strings.TrimSpace(base))
	}

	return defaultMediaType
}
