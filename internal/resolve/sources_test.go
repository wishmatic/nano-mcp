package resolve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/wishmatic/nano-mcp/internal/sourcemap"
)

func newMappedResolver(t *testing.T, spec string) *Client {
	t.Helper()

	sources, err := sourcemap.Parse(spec)
	if err != nil {
		t.Fatalf("sourcemap.Parse(%q) error: %v", spec, err)
	}

	return New(sources)
}

func TestFetchRewritesMappedHost(t *testing.T) {
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("from-private:" + r.URL.Path))
	}))
	t.Cleanup(private.Close)

	r := newMappedResolver(t, "https://images.example.com="+private.URL)

	data, err := r.Fetch(context.Background(), "https://images.example.com/i/x.webp")
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if want := "from-private:/i/x.webp"; string(data) != want {
		t.Errorf("Fetch() = %q, want %q", data, want)
	}
}

func TestFetchRewritesMappedRedirectTarget(t *testing.T) {
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("from-private"))
	}))
	t.Cleanup(private.Close)

	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://images.example.com/i/y.webp", http.StatusFound)
	}))
	t.Cleanup(public.Close)

	r := newMappedResolver(t, "https://images.example.com="+private.URL)

	data, err := r.Fetch(context.Background(), public.URL+"/x.webp")
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if string(data) != "from-private" {
		t.Errorf("Fetch() = %q, want the mapped private response", data)
	}
}

func TestFetchReadsMappedDirectory(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "x.png"), []byte("from-disk"), 0o640); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	r := newMappedResolver(t, "https://chat.example.com/images/="+dir)

	data, err := r.Fetch(context.Background(), "https://chat.example.com/images/x.png")
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if string(data) != "from-disk" {
		t.Errorf("Fetch() = %q, want from-disk", data)
	}
}

func TestFetchRefusesMappedDirectoryEscapes(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "x.png"), []byte("from-disk"), 0o640); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	r := newMappedResolver(t, "https://chat.example.com/images/="+dir)

	tests := map[string]string{
		"traversal":    "https://chat.example.com/images/../../etc/passwd.png",
		"unknown type": "https://chat.example.com/images/notes.txt",
		"missing":      "https://chat.example.com/images/missing.png",
	}

	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := r.Fetch(context.Background(), target); err == nil {
				t.Errorf("Fetch(%s) error = nil, want an error", target)
			}
		})
	}
}

func TestFetchWithoutSourcesIsUnchanged(t *testing.T) {
	r := newMappedResolver(t, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("plain"))
	}))
	t.Cleanup(server.Close)

	data, err := r.Fetch(context.Background(), server.URL+"/x.webp")
	if err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}

	if string(data) != "plain" {
		t.Errorf("Fetch() = %q, want plain", data)
	}
}
