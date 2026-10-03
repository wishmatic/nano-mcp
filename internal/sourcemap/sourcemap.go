package sourcemap

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxFileBytes = 32 << 20

var readableExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".jxl":  true,
}

type Kind int

const (
	Directory Kind = iota
	BaseURL
)

type Source struct {
	kind Kind
	base string
	dir  string
}

func (s Source) Kind() Kind {
	return s.kind
}

// Target returns the URL to fetch in place of u, which is only meaningful for a base URL source.
func (s Source) Target(u *url.URL, rest string) (*url.URL, error) {
	if s.kind != BaseURL {
		return nil, errors.New("sourcemap: only a base URL source can rewrite a URL")
	}

	raw := s.base
	if rest != "" {
		raw += "/" + rest
	}

	if u.RawQuery != "" {
		raw += "?" + u.RawQuery
	}

	target, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("sourcemap: rewritten URL %q: %w", raw, err)
	}

	return target, nil
}

// Read returns the file a directory source maps rest to.
func (s Source) Read(rest string) ([]byte, error) {
	path, err := s.FilePath(rest)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("sourcemap: stat %s: %w", rest, err)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sourcemap: %s is not a regular file", rest)
	}

	if info.Size() > maxFileBytes {
		return nil, fmt.Errorf("sourcemap: %s is %d bytes, over the %d byte limit", rest, info.Size(), maxFileBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sourcemap: read %s: %w", rest, err)
	}

	return data, nil
}

// FilePath validates rest as a readable image path under the source's directory.
func (s Source) FilePath(rest string) (string, error) {
	rel, err := url.PathUnescape(rest)
	if err != nil {
		return "", fmt.Errorf("sourcemap: unescape %q: %w", rest, err)
	}

	if rel == "" {
		return "", errors.New("sourcemap: empty file path")
	}

	if strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("sourcemap: %q is an absolute path", rel)
	}

	if !readableExtensions[strings.ToLower(filepath.Ext(rel))] {
		return "", fmt.Errorf("sourcemap: %q is not a readable image type", rel)
	}

	full := filepath.Join(s.dir, filepath.FromSlash(rel))

	within, err := filepath.Rel(s.dir, full)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("sourcemap: %q escapes %s", rel, s.dir)
	}

	return full, nil
}

type Map struct {
	entries []entry
}

type entry struct {
	scheme string
	host   string
	path   string
	source Source
}

// Parse reads a comma-separated list of public=private pairs, where the private side is either an absolute http(s) base
// URL or a directory to read from.
func Parse(spec string) (*Map, error) {
	m := &Map{}
	seen := make(map[string]bool)

	for _, pair := range strings.Split(spec, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		public, private, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("sourcemap: entry %q is not a public=private pair", pair)
		}

		key, err := parsePublic(strings.TrimSpace(public))
		if err != nil {
			return nil, err
		}

		signature := key.scheme + "://" + key.host + key.path
		if seen[signature] {
			return nil, fmt.Errorf("sourcemap: %s is mapped more than once", signature)
		}

		seen[signature] = true

		source, err := parsePrivate(strings.TrimSpace(private))
		if err != nil {
			return nil, err
		}

		key.source = source
		m.entries = append(m.entries, key)
	}

	return m, nil
}

func parsePublic(public string) (entry, error) {
	parsed, err := url.Parse(public)
	if err != nil {
		return entry{}, fmt.Errorf("sourcemap: public URL %q: %w", public, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return entry{}, fmt.Errorf("sourcemap: public URL %q must use http or https", public)
	}

	if parsed.Host == "" {
		return entry{}, fmt.Errorf("sourcemap: public URL %q must include a host", public)
	}

	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return entry{}, fmt.Errorf("sourcemap: public URL %q must not include a query or fragment", public)
	}

	return entry{
		scheme: strings.ToLower(parsed.Scheme),
		host:   strings.ToLower(parsed.Host),
		path:   strings.TrimSuffix(parsed.EscapedPath(), "/"),
	}, nil
}

func parsePrivate(private string) (Source, error) {
	if strings.HasPrefix(private, "http://") || strings.HasPrefix(private, "https://") {
		parsed, err := url.Parse(private)
		if err != nil {
			return Source{}, fmt.Errorf("sourcemap: private URL %q: %w", private, err)
		}

		if parsed.Host == "" {
			return Source{}, fmt.Errorf("sourcemap: private URL %q must include a host", private)
		}

		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return Source{}, fmt.Errorf("sourcemap: private URL %q must not include a query or fragment", private)
		}

		return Source{kind: BaseURL, base: strings.TrimSuffix(private, "/")}, nil
	}

	if !filepath.IsAbs(private) {
		return Source{}, fmt.Errorf("sourcemap: %q is neither an http(s) URL nor an absolute directory", private)
	}

	return Source{kind: Directory, dir: filepath.Clean(private)}, nil
}

// Lookup returns the source for u and the URL path remainder the source maps, choosing the longest matching prefix.
func (m *Map) Lookup(u *url.URL) (Source, string, bool) {
	if m == nil {
		return Source{}, "", false
	}

	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Host)
	target := u.EscapedPath()

	best := -1

	var (
		source Source
		rest   string
	)

	for _, e := range m.entries {
		if e.scheme != scheme || e.host != host || len(e.path) <= best {
			continue
		}

		if !hasPathPrefix(target, e.path) {
			continue
		}

		best = len(e.path)
		source = e.source
		rest = strings.TrimPrefix(strings.TrimPrefix(target, e.path), "/")
	}

	if best < 0 {
		return Source{}, "", false
	}

	return source, rest, true
}

func hasPathPrefix(target, prefix string) bool {
	if prefix == "" {
		return true
	}

	return target == prefix || strings.HasPrefix(target, prefix+"/")
}
