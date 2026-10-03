package filestore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

const testPublicHost = "https://cdn.example.com"

func TestNewRejectsAnIncompleteConfig(t *testing.T) {
	base, err := url.Parse(testPublicHost)
	if err != nil {
		t.Fatalf("url.Parse() error: %v", err)
	}

	tests := map[string]Config{
		"no directory":               {PublicBase: base},
		"no public base":             {Dir: t.TempDir()},
		"public base without a host": {Dir: t.TempDir(), PublicBase: &url.URL{Scheme: "https"}},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := New(cfg, zap.NewNop()); err == nil {
				t.Error("New() error = nil, want a rejection")
			}
		})
	}
}

func TestNewCreatesTheStorageDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "files")

	if _, err := New(Config{Dir: dir, PublicBase: testBase(t)}, zap.NewNop()); err != nil {
		t.Fatalf("New() error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Errorf("Stat(%s) = %v, %v, want a directory", dir, info, err)
	}
}

func TestPutWritesTheAssetAndReturnsItsLink(t *testing.T) {
	client, dir := testClient(t)

	asset, err := client.Put(context.Background(), []byte("video bytes"), "video/mp4")
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	if !strings.HasPrefix(asset.URL, testPublicHost+"/v/") || !strings.HasSuffix(asset.URL, ".mp4") {
		t.Errorf("URL = %q, want a dated mp4 under the public host", asset.URL)
	}

	if asset.MediaType != "video/mp4" || asset.Bytes != int64(len("video bytes")) {
		t.Errorf("asset = %+v, want the media type and the byte count", asset)
	}

	stored, err := os.ReadFile(pathOf(t, dir, asset.URL))
	if err != nil {
		t.Fatalf("read stored asset: %v", err)
	}

	if string(stored) != "video bytes" {
		t.Errorf("stored = %q, want the uploaded bytes", stored)
	}
}

func TestPutNamesEachAssetApart(t *testing.T) {
	client, _ := testClient(t)

	first, err := client.Put(context.Background(), []byte("a"), "video/mp4")
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	second, err := client.Put(context.Background(), []byte("b"), "video/mp4")
	if err != nil {
		t.Fatalf("Put() error: %v", err)
	}

	if first.URL == second.URL {
		t.Errorf("both assets landed at %q, want distinct keys", first.URL)
	}
}

func TestPutRejectsAMediaTypeItDoesNotServe(t *testing.T) {
	client, _ := testClient(t)

	if _, err := client.Put(context.Background(), []byte("x"), "application/pdf"); err == nil {
		t.Error("Put() error = nil, want a rejection rather than a file nothing can serve")
	}
}

func TestPutHonoursACancelledContext(t *testing.T) {
	client, _ := testClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Put(ctx, []byte("x"), "video/mp4"); err == nil {
		t.Error("Put() error = nil, want the cancellation")
	}
}

func TestFetchCopiesTheAsset(t *testing.T) {
	client, dir := testClient(t)

	source := serve(t, "video/mp4", "fetched bytes")

	asset, err := client.Fetch(context.Background(), source)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if !strings.HasSuffix(asset.URL, ".mp4") || asset.MediaType != "video/mp4" {
		t.Errorf("asset = %+v, want the source's media type", asset)
	}

	stored, err := os.ReadFile(pathOf(t, dir, asset.URL))
	if err != nil {
		t.Fatalf("read stored asset: %v", err)
	}

	if string(stored) != "fetched bytes" {
		t.Errorf("stored = %q, want the fetched bytes", stored)
	}
}

func TestFetchUsesTheDefaultTypeWhenTheSourceHasNone(t *testing.T) {
	client, _ := testClient(t)

	source := serve(t, "application/octet-stream", "bytes")

	asset, err := client.Fetch(context.Background(), source)
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if asset.MediaType != defaultMediaType || !strings.HasSuffix(asset.URL, ".mp4") {
		t.Errorf("asset = %+v, want it stored as %s", asset, defaultMediaType)
	}
}

func TestFetchReportsAFailedDownload(t *testing.T) {
	client, _ := testClient(t)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(source.Close)

	_, err := client.Fetch(context.Background(), source.URL)
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("Fetch() error = %v, want the upstream status surfaced", err)
	}
}

func TestFetchRejectsASourceItCannotGet(t *testing.T) {
	client, _ := testClient(t)

	for _, source := range []string{"file:///etc/passwd", "not a url at all"} {
		if _, err := client.Fetch(context.Background(), source); err == nil {
			t.Errorf("Fetch(%q) error = nil, want a rejection", source)
		}
	}
}

func TestFetchRejectsAnAssetOverTheCap(t *testing.T) {
	client, _ := testClient(t)
	client.maxBytes = 8

	source := serve(t, "video/mp4", "nine byte")

	_, err := client.Fetch(context.Background(), source)
	if err == nil || !strings.Contains(err.Error(), "cap") {
		t.Errorf("Fetch() error = %v, want the cap named", err)
	}
}

func serve(t *testing.T, contentType, body string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

func testBase(t *testing.T) *url.URL {
	t.Helper()

	base, err := url.Parse(testPublicHost)
	if err != nil {
		t.Fatalf("url.Parse() error: %v", err)
	}

	return base
}

func testClient(t *testing.T) (*Client, string) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "files")

	client, err := New(Config{Dir: dir, PublicBase: testBase(t)}, zap.NewNop())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	return client, dir
}

func pathOf(t *testing.T, dir, assetURL string) string {
	t.Helper()

	parsed, err := url.Parse(assetURL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error: %v", assetURL, err)
	}

	return filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(parsed.Path, "/")))
}
