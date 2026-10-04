package filestore

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

// The types this server keeps and serves. Anything else is refused rather than written, so a
// mislabelled download cannot become a file with a type nothing declares.
const defaultMediaType = "video/mp4"

var mediaTypes = map[string]string{
	"video/mp4":       "mp4",
	"video/webm":      "webm",
	"video/quicktime": "mov",

	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/webp": "webp",
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

// DetectMediaType names the type of an asset from its own bytes, for a source that labelled them as
// nothing in particular or, when an image arrives inline, not at all. Only the types it serves are
// reported, which is what makes a false a refusal rather than a missed guess.
func DetectMediaType(data []byte) (string, bool) {
	base, _, _ := strings.Cut(http.DetectContentType(data), ";")

	return servedMediaType(base)
}

// mediaTypeOf trusts a provider's content type when it names a format we serve, reads the bytes when
// it does not, and falls back to video/mp4 for a download that is neither labelled nor recognisable,
// which a video off a mislabelling provider usually is.
func mediaTypeOf(header string, data []byte) string {
	base, _, _ := strings.Cut(header, ";")

	if mediaType, ok := servedMediaType(base); ok {
		return mediaType
	}

	if mediaType, ok := DetectMediaType(data); ok {
		return mediaType
	}

	return defaultMediaType
}

func servedMediaType(candidate string) (string, bool) {
	mediaType := strings.ToLower(strings.TrimSpace(candidate))

	_, ok := mediaTypes[mediaType]

	return mediaType, ok
}
