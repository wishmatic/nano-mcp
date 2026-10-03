package sourcemap

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustParse(t *testing.T, spec string) *Map {
	t.Helper()

	m, err := Parse(spec)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", spec, err)
	}

	return m
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) error: %v", raw, err)
	}

	return u
}

func TestParseEmptyMatchesNothing(t *testing.T) {
	for _, spec := range []string{"", " ", ",", " , "} {
		m := mustParse(t, spec)

		if _, _, ok := m.Lookup(mustURL(t, "https://example.com/i/x.webp")); ok {
			t.Errorf("Parse(%q) matched a URL, want no entries", spec)
		}
	}
}

func TestParseRejectsBadEntries(t *testing.T) {
	tests := map[string]string{
		"no separator":       "https://example.com",
		"empty private":      "https://example.com=",
		"empty public":       "=/var/images",
		"missing scheme":     "example.com=/var/images",
		"wrong scheme":       "ftp://example.com=/var/images",
		"relative directory": "https://example.com=images",
		"query in private":   "https://example.com=http://librechat:3080/x?a=1",
		"duplicate":          "https://example.com=/var/a,https://example.com=/var/b",
	}

	for name, spec := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(spec); err == nil {
				t.Errorf("Parse(%q) error = nil, want an error", spec)
			}
		})
	}
}

func TestParseAcceptsBothKinds(t *testing.T) {
	m := mustParse(t, "https://example.com/images/=/var/images, https://neo.example.com=http://neo-mcp:8080")

	directory, rest, ok := m.Lookup(mustURL(t, "https://example.com/images/2026-09/a.png"))
	if !ok || directory.Kind() != Directory {
		t.Fatalf("Lookup() = %v, %v, want a directory source", directory.Kind(), ok)
	}

	if rest != "2026-09/a.png" {
		t.Errorf("remainder = %q, want 2026-09/a.png", rest)
	}

	base, rest, ok := m.Lookup(mustURL(t, "https://neo.example.com/i/x.webp"))
	if !ok || base.Kind() != BaseURL {
		t.Fatalf("Lookup() = %v, %v, want a base URL source", base.Kind(), ok)
	}

	if rest != "i/x.webp" {
		t.Errorf("remainder = %q, want i/x.webp", rest)
	}
}

func TestLookupLongestPathWins(t *testing.T) {
	m := mustParse(t, "https://example.com=http://internal:8080,https://example.com/images/=/var/images")

	tests := []struct {
		name string
		url  string
		want Kind
		rest string
	}{
		{name: "host wide", url: "https://example.com/i/x.webp", want: BaseURL, rest: "i/x.webp"},
		{name: "path specific", url: "https://example.com/images/a.png", want: Directory, rest: "a.png"},
		{name: "path specific nested", url: "https://example.com/images/2026-09/a.png", want: Directory, rest: "2026-09/a.png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, rest, ok := m.Lookup(mustURL(t, tt.url))
			if !ok {
				t.Fatalf("Lookup(%s) = no match, want a source", tt.url)
			}

			if source.Kind() != tt.want {
				t.Errorf("kind = %v, want %v", source.Kind(), tt.want)
			}

			if rest != tt.rest {
				t.Errorf("remainder = %q, want %q", rest, tt.rest)
			}
		})
	}
}

func TestLookupBoundaries(t *testing.T) {
	m := mustParse(t, "https://example.com/img=/var/images")

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "exact prefix", url: "https://example.com/img", want: true},
		{name: "child of prefix", url: "https://example.com/img/a.png", want: true},
		{name: "string prefix only", url: "https://example.com/images/a.png", want: false},
		{name: "host case insensitive", url: "https://EXAMPLE.com/img/a.png", want: true},
		{name: "other host", url: "https://other.com/img/a.png", want: false},
		{name: "other scheme", url: "http://example.com/img/a.png", want: false},
		{name: "other port", url: "https://example.com:8443/img/a.png", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, ok := m.Lookup(mustURL(t, tt.url)); ok != tt.want {
				t.Errorf("Lookup(%s) matched = %v, want %v", tt.url, ok, tt.want)
			}
		})
	}
}

func TestTargetBuildsPrivateURL(t *testing.T) {
	tests := []struct {
		name string
		spec string
		url  string
		want string
	}{
		{
			name: "host wide",
			spec: "https://neo.example.com=http://neo-mcp:8080",
			url:  "https://neo.example.com/i/x.webp",
			want: "http://neo-mcp:8080/i/x.webp",
		},
		{
			name: "path prefix",
			spec: "https://example.com/images/=http://librechat:3080/images",
			url:  "https://example.com/images/2026-09/a.png",
			want: "http://librechat:3080/images/2026-09/a.png",
		},
		{
			name: "query preserved",
			spec: "https://neo.example.com=http://neo-mcp:8080",
			url:  "https://neo.example.com/i/x.webp?sig=abc",
			want: "http://neo-mcp:8080/i/x.webp?sig=abc",
		},
		{
			name: "no remainder",
			spec: "https://neo.example.com=http://neo-mcp:8080",
			url:  "https://neo.example.com",
			want: "http://neo-mcp:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := mustParse(t, tt.spec)
			u := mustURL(t, tt.url)

			source, rest, ok := m.Lookup(u)
			if !ok {
				t.Fatalf("Lookup(%s) = no match, want a source", tt.url)
			}

			target, err := source.Target(u, rest)
			if err != nil {
				t.Fatalf("Target() error: %v", err)
			}

			if got := target.String(); got != tt.want {
				t.Errorf("Target() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilePathRefusals(t *testing.T) {
	dir := t.TempDir()
	source := Source{kind: Directory, dir: dir}

	tests := map[string]string{
		"empty":            "",
		"absolute":         "/etc/passwd.png",
		"traversal":        "../../etc/passwd.png",
		"nested traversal": "a/../../x.png",
		"unknown type":     "notes.txt",
		"no extension":     "notes",
	}

	for name, rest := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := source.FilePath(rest); err == nil {
				t.Errorf("FilePath(%q) error = nil, want an error", rest)
			}
		})
	}
}

func TestFilePathAcceptsNestedImage(t *testing.T) {
	dir := t.TempDir()
	source := Source{kind: Directory, dir: dir}

	got, err := source.FilePath("2026-09/abc.png")
	if err != nil {
		t.Fatalf("FilePath() error: %v", err)
	}

	if want := filepath.Join(dir, "2026-09", "abc.png"); got != want {
		t.Errorf("FilePath() = %q, want %q", got, want)
	}
}

func TestReadReturnsFileContents(t *testing.T) {
	dir := t.TempDir()
	source := Source{kind: Directory, dir: dir}

	if err := os.MkdirAll(filepath.Join(dir, "2026-09"), 0o750); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}

	name := filepath.Join(dir, "2026-09", "a b.png")
	if err := os.WriteFile(name, []byte("image-bytes"), 0o640); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	data, err := source.Read("2026-09/a%20b.png")
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	if string(data) != "image-bytes" {
		t.Errorf("Read() = %q, want image-bytes", data)
	}
}

func TestReadRefusals(t *testing.T) {
	dir := t.TempDir()
	source := Source{kind: Directory, dir: dir}

	if err := os.Mkdir(filepath.Join(dir, "dir.png"), 0o750); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}

	oversized := filepath.Join(dir, "big.png")
	if err := os.Truncate(oversized, maxFileBytes+1); err != nil {
		// Truncate cannot create the file, so write it sparse instead.
		if err := os.WriteFile(oversized, nil, 0o640); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}

		if err := os.Truncate(oversized, maxFileBytes+1); err != nil {
			t.Fatalf("Truncate() error: %v", err)
		}
	}

	tests := map[string]string{
		"missing file": "missing.png",
		"directory":    "dir.png",
		"oversized":    "big.png",
		"traversal":    "../escape.png",
	}

	for name, rest := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := source.Read(rest); err == nil {
				t.Errorf("Read(%q) error = nil, want an error", rest)
			}
		})
	}
}

func TestParseTrimsWhitespace(t *testing.T) {
	m := mustParse(t, "  https://example.com = /var/images  ")

	source, rest, ok := m.Lookup(mustURL(t, "https://example.com/a.png"))
	if !ok {
		t.Fatal("Lookup() = no match, want a source")
	}

	if source.Kind() != Directory || rest != "a.png" {
		t.Errorf("Lookup() = kind %v, rest %q, want a directory source and a.png", source.Kind(), rest)
	}

	if !strings.HasPrefix(source.dir, string(filepath.Separator)) {
		t.Errorf("dir = %q, want an absolute directory", source.dir)
	}
}
