package filestore

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestServeReturnsTheStoredAsset(t *testing.T) {
	client, _ := testClient(t)

	asset, err := client.Put(context.Background(), []byte("video bytes"), "video/mp4")
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	rec := get(t, client, http.MethodGet, asset.URL)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if got := rec.Body.String(); got != "video bytes" {
		t.Errorf("body = %q, want the stored bytes", got)
	}

	if got := rec.Header().Get("Content-Type"); got != "video/mp4" {
		t.Errorf("Content-Type = %q, want video/mp4", got)
	}

	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q, want the asset cached forever", got)
	}

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestServeAnswersHead(t *testing.T) {
	client, _ := testClient(t)

	asset, err := client.Put(context.Background(), []byte("video bytes"), "video/webm")
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	rec := get(t, client, http.MethodHead, asset.URL)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if got := rec.Body.String(); got != "" {
		t.Errorf("body = %q, want none for a HEAD", got)
	}
}

func TestServeReturnsAnImage(t *testing.T) {
	client, _ := testClient(t)

	asset, err := client.Put(context.Background(), testPNG, "image/png")
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	rec := get(t, client, http.MethodGet, asset.URL)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if !bytes.Equal(rec.Body.Bytes(), testPNG) {
		t.Errorf("body = %q, want the stored bytes", rec.Body.Bytes())
	}

	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", got)
	}
}

func TestDetectMediaTypeNamesWhatItServes(t *testing.T) {
	tests := map[string]struct {
		data []byte
		want string
	}{
		"png":     {data: testPNG, want: "image/png"},
		"jpeg":    {data: []byte("\xff\xd8\xff and more"), want: "image/jpeg"},
		"webp":    {data: []byte("RIFF\x00\x00\x00\x00WEBPVP8 and more"), want: "image/webp"},
		"mp4":     {data: []byte("\x00\x00\x00\x14ftypmp42\x00\x00\x00\x00mp42"), want: "video/mp4"},
		"webm":    {data: []byte("\x1a\x45\xdf\xa3 and more"), want: "video/webm"},
		"not one": {data: []byte("plain text, most likely")},
		"empty":   {data: nil},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := DetectMediaType(tt.data)
			if tt.want == "" {
				if ok {
					t.Errorf("DetectMediaType() = %q, true, want it refused", got)
				}

				return
			}

			if !ok || got != tt.want {
				t.Errorf("DetectMediaType() = %q, %v, want %q", got, ok, tt.want)
			}
		})
	}
}

func TestServeHidesWhatItCannotServe(t *testing.T) {
	client, _ := testClient(t)

	asset, err := client.Put(context.Background(), []byte("video bytes"), "video/mp4")
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	paths := map[string]string{
		"an extension with no media type": swapExtension(t, asset.URL, ".pdf"),
		"a key that was never stored":     swapExtension(t, asset.URL, ".mp4x"),
		"a path outside the namespace":    "/secret.mp4",
		"a traversing key":                "/v/../secret.mp4",
	}

	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			client.served().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d for %s, want 404", rec.Code, path)
			}
		})
	}
}

func get(t *testing.T, client *Client, method, assetURL string) *httptest.ResponseRecorder {
	t.Helper()

	parsed, err := url.Parse(assetURL)
	if err != nil {
		t.Fatalf("url.Parse() error: %v", err)
	}

	rec := httptest.NewRecorder()
	client.served().ServeHTTP(rec, httptest.NewRequest(method, parsed.Path, nil))

	return rec
}

func swapExtension(t *testing.T, assetURL, ext string) string {
	t.Helper()

	parsed, err := url.Parse(assetURL)
	if err != nil {
		t.Fatalf("url.Parse() error: %v", err)
	}

	return strings.TrimSuffix(parsed.Path, filepath.Ext(parsed.Path)) + ext
}

// served mounts the file routes the way a server does, so tests exercise the same router the
// traversal guard has to hold under.
func (c *Client) served() chi.Router {
	router := chi.NewRouter()
	c.Register(router)

	return router
}
