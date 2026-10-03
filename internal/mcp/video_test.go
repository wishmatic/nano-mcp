package mcp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testVideoBytes = "mpeg bytes"

func TestGenerateVideoToolIsListed(t *testing.T) {
	session := nanoSession(t, "http://127.0.0.1:1")

	props := toolProperties(t, findTool(t, session, "generate_video"))

	want := []string{
		"model", "prompt", "negativePrompt", "duration", "aspectRatio", "resolution", "seed",
		"generateAudio", "imageUrl", "videoUrl",
	}

	for _, name := range want {
		if _, ok := props[name]; !ok {
			t.Errorf("the input schema has no %q", name)
		}
	}
}

func TestGenerateVideoStoresWhatItGenerates(t *testing.T) {
	video := serveVideo(t)

	nano := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"runId":"vid_9","status":"pending"}`))

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"status":"COMPLETED","cost":0.2,"output":{"video":{"url":"` +
			video.URL + `/out.mp4"}}}}`))
	})

	store, dir := videoStore(t)

	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, nano)
	deps.Files = store

	result := callTool(t, connectedSession(t, deps), "generate_video", map[string]any{
		"model":  "veo2-video",
		"prompt": "a lake at sunset",
	})

	if result.IsError {
		t.Fatalf("generate_video failed: %s", errorText(result))
	}

	if len(result.Content) != 1 {
		t.Fatalf("content = %#v, want only the link to the hosted copy", result.Content)
	}

	link, ok := result.Content[0].(*mcp.ResourceLink)
	if !ok {
		t.Fatalf("content = %T, want a resource link rather than a media block", result.Content[0])
	}

	if link.MIMEType != "video/mp4" || link.Size == nil || *link.Size != int64(len(testVideoBytes)) {
		t.Errorf("link = %+v, want the copied media type and size", link)
	}

	var out struct {
		RunID       string  `json:"runId"`
		Model       string  `json:"model"`
		VideoURL    string  `json:"videoUrl"`
		ContentType string  `json:"contentType"`
		Bytes       int64   `json:"bytes"`
		SourceURL   string  `json:"sourceUrl"`
		CostUSD     float64 `json:"costUsd"`
	}
	structured(t, result, &out)

	if out.RunID != "vid_9" || out.Model != "veo2-video" || out.CostUSD != 0.2 {
		t.Errorf("output = %+v, want the job, the model, and the charged cost", out)
	}

	if out.ContentType != "video/mp4" || out.Bytes != int64(len(testVideoBytes)) {
		t.Errorf("output = %+v, want the copied media type and size", out)
	}

	if out.SourceURL != video.URL+"/out.mp4" {
		t.Errorf("sourceUrl = %q, want the provider URL", out.SourceURL)
	}

	parsed, err := url.Parse(out.VideoURL)
	if err != nil {
		t.Fatalf("url.Parse() error: %v", err)
	}

	if parsed.Host != "nano.example.com" || !strings.HasPrefix(parsed.Path, "/v/") {
		t.Errorf("videoUrl = %q, want it served from the public host", out.VideoURL)
	}

	if link.URI != out.VideoURL {
		t.Errorf("link.URI = %q, want the same URL as the output's %q", link.URI, out.VideoURL)
	}

	key := filepath.FromSlash(strings.TrimPrefix(parsed.Path, "/"))

	copied, err := os.ReadFile(filepath.Join(dir, key))
	if err != nil {
		t.Fatalf("read the hosted copy: %v", err)
	}

	if string(copied) != testVideoBytes {
		t.Errorf("hosted copy = %q, want the provider's bytes", copied)
	}
}

func TestGenerateVideoReportsAnUpstreamRefusal(t *testing.T) {
	nano := upstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"message":"out of credit"}`))
	})

	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, nano)
	deps.Files = fileStore(t)

	result := callTool(t, connectedSession(t, deps), "generate_video", map[string]any{
		"model":  "veo2-video",
		"prompt": "a lake",
	})

	if !result.IsError {
		t.Fatal("generate_video succeeded, want the refusal surfaced")
	}

	text := errorText(result)
	if !strings.Contains(text, "402") || !strings.Contains(text, "out of credit") {
		t.Errorf("error = %q, want the status and the message", text)
	}
}

func TestGenerateVideoNeedsAModelAndPrompt(t *testing.T) {
	store, _ := videoStore(t)

	for name, args := range map[string]map[string]any{
		"no model":  {"prompt": "a lake"},
		"no prompt": {"model": "veo2-video"},
	} {
		t.Run(name, func(t *testing.T) {
			deps := noopDeps()
			deps.NanoGPT = nanoClient(t, "http://127.0.0.1:1")
			deps.Files = store

			result := callTool(t, connectedSession(t, deps), "generate_video", args)

			if !result.IsError || !strings.Contains(errorText(result), "required") {
				t.Errorf("result = %#v, want the missing field named", result.Content)
			}
		})
	}
}

func serveVideo(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte(testVideoBytes))
	}))
	t.Cleanup(srv.Close)

	return srv
}
