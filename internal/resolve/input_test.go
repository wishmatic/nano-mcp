package resolve

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var (
	pngBytes  = []byte("\x89PNG\r\n\x1a\n and more")
	jpegBytes = []byte("\xff\xd8\xff\xe0 and more")
	webpBytes = []byte("RIFF\x00\x00\x00\x00WEBP and more")
	gifBytes  = []byte("GIF89a and more")
)

func encodeDataURI(mediaType string, data []byte) string {
	return "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func insertWhitespace(s string) string {
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && i%4 == 0 {
			b.WriteByte('\n')
		}

		b.WriteRune(ch)
	}

	return b.String()
}

func TestResolveInputForms(t *testing.T) {
	rawPNG := base64.StdEncoding.EncodeToString(pngBytes)

	tests := []struct {
		name      string
		input     string
		wantMedia string
		wantData  []byte
	}{
		{"png data uri", encodeDataURI(MediaPNG, pngBytes), MediaPNG, pngBytes},
		{"jpeg data uri", encodeDataURI(MediaJPEG, jpegBytes), MediaJPEG, jpegBytes},
		{"webp data uri", encodeDataURI(MediaWebP, webpBytes), MediaWebP, webpBytes},
		{"raw base64 png", rawPNG, MediaPNG, pngBytes},
		{"unpadded base64", base64.RawStdEncoding.EncodeToString(pngBytes), MediaPNG, pngBytes},
		{"url safe base64", base64.RawURLEncoding.EncodeToString(jpegBytes), MediaJPEG, jpegBytes},
		{"base64 with whitespace", insertWhitespace(rawPNG), MediaPNG, pngBytes},
		{"surrounding whitespace", "  " + rawPNG + "\n", MediaPNG, pngBytes},
		{"uppercase declared type", encodeDataURI("IMAGE/PNG", pngBytes), MediaPNG, pngBytes},
		{
			"declared type fallback",
			"data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not an image")),
			MediaPNG,
			[]byte("not an image"),
		},
	}

	r := newResolver(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := r.Resolve(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}

			if img.MediaType != tt.wantMedia {
				t.Errorf("MediaType = %q, want %q", img.MediaType, tt.wantMedia)
			}

			if !bytes.Equal(img.Data, tt.wantData) {
				t.Errorf("Data = %q, want %q", img.Data, tt.wantData)
			}
		})
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSubstr string
	}{
		{"empty", "   ", "empty image input"},
		{"data uri missing comma", "data:image/png;base64", "missing comma"},
		{"data uri not base64", "data:image/png,abc", "only base64"},
		{"invalid base64", "%%%not-base64%%%", "invalid base64"},
		{"a gif is not a video model's input", encodeDataURI("image/gif", gifBytes), "image/gif"},
		{"unsupported gif raw", base64.StdEncoding.EncodeToString(gifBytes), "image/gif"},
		{"plain text", base64.StdEncoding.EncodeToString([]byte("just some words, here")), "text/plain"},
	}

	r := newResolver(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.Resolve(context.Background(), tt.input)
			if err == nil {
				t.Fatal("Resolve() expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantSubstr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantSubstr)
			}

			if !strings.HasPrefix(err.Error(), "resolve:") {
				t.Errorf("error = %q, want resolve prefix", err.Error())
			}
		})
	}
}

func TestResolveFetchesURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(pngBytes)
	}))
	defer server.Close()

	img, err := newResolver(t).Resolve(context.Background(), server.URL+"/x.png")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	if img.MediaType != MediaPNG {
		t.Errorf("MediaType = %q, want %q", img.MediaType, MediaPNG)
	}

	if !bytes.Equal(img.Data, pngBytes) {
		t.Errorf("Data = %q, want %q", img.Data, pngBytes)
	}
}

func TestResolveFollowsMultipleRedirects(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(pngBytes)
	}))
	defer origin.Close()

	hop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, origin.URL+"/x.png", http.StatusFound)
	}))
	defer hop.Close()

	start := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, hop.URL, http.StatusMovedPermanently)
	}))
	defer start.Close()

	img, err := newResolver(t).Resolve(context.Background(), start.URL)
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	if !bytes.Equal(img.Data, pngBytes) {
		t.Errorf("Data = %q, want %q", img.Data, pngBytes)
	}
}

func TestResolveRejectsAnImageOverTheLimit(t *testing.T) {
	r := newResolver(t)
	r.maxBytes = 4

	_, err := r.Resolve(context.Background(), encodeDataURI(MediaPNG, pngBytes))
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Errorf("Resolve() error = %v, want the limit named", err)
	}
}
