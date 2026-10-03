package mcp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/resolve"
)

const testVideoBytes = "mpeg bytes"

var testPNGBytes = []byte("\x89PNG\r\n\x1a\n and more")

func TestGenerateVideoToolIsListed(t *testing.T) {
	session := nanoSession(t, "http://127.0.0.1:1")

	props := toolProperties(t, findTool(t, session, "generate_video"))

	want := []string{
		"model", "prompt", "image", "negativePrompt", "duration", "aspectRatio", "resolution", "seed",
		"generateAudio", "videoUrl",
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
	deps.Resolver = addressMap(t, "")

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
	deps.Resolver = addressMap(t, "")

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
			deps.Resolver = addressMap(t, "")

			result := callTool(t, connectedSession(t, deps), "generate_video", args)

			if !result.IsError || !strings.Contains(errorText(result), "required") {
				t.Errorf("result = %#v, want the missing field named", result.Content)
			}
		})
	}
}

func TestGenerateVideoAnimatesAMappedImage(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "start.png"), testPNGBytes, 0o640); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	nano, submits := recordingNano(t, serveVideo(t).URL+"/out.mp4")

	result := generateVideo(t, nano, addressMap(t, "https://chat.example.com/images/="+dir), map[string]any{
		"model":  "veo2-video",
		"prompt": "a lake at sunset",
		"image":  "https://chat.example.com/images/start.png",
	})

	if result.IsError {
		t.Fatalf("generate_video failed: %s", errorText(result))
	}

	want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNGBytes)

	body := submits.body(t)
	if body["imageDataUrl"] != want {
		t.Errorf("imageDataUrl = %v, want the resolved bytes as a data URL", body["imageDataUrl"])
	}

	if body["mode"] != "image-to-video" {
		t.Errorf("mode = %v, want an image to switch the call to image-to-video", body["mode"])
	}
}

func TestGenerateVideoKeepsAModeTheCallerNamed(t *testing.T) {
	nano, submits := recordingNano(t, serveVideo(t).URL+"/out.mp4")

	result := generateVideo(t, nano, addressMap(t, ""), map[string]any{
		"model":  "veo2-video",
		"prompt": "a lake",
		"image":  "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPNGBytes),
		"mode":   "reference-to-video",
	})

	if result.IsError {
		t.Fatalf("generate_video failed: %s", errorText(result))
	}

	if body := submits.body(t); body["mode"] != "reference-to-video" {
		t.Errorf("mode = %v, want the caller's own mode kept", body["mode"])
	}
}

func TestGenerateVideoRefusesAnImageItCannotUse(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, upstream(t, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("the upstream was called for an image that cannot be resolved: %s", r.URL.Path)
	}))
	deps.Files = fileStore(t)
	deps.Resolver = addressMap(t, "")

	result := callTool(t, connectedSession(t, deps), "generate_video", map[string]any{
		"model":  "veo2-video",
		"prompt": "a lake",
		"image":  "data:image/gif;base64," + base64.StdEncoding.EncodeToString([]byte("GIF89a")),
	})

	if !result.IsError || !strings.Contains(errorText(result), "image/gif") {
		t.Errorf("result = %#v, want the unsupported type named", result.Content)
	}
}

func generateVideo(t *testing.T, nano string, resolver *resolve.Client, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, nano)
	deps.Files = fileStore(t)
	deps.Resolver = resolver

	return callTool(t, connectedSession(t, deps), "generate_video", args)
}

// submitRecorder keeps the body of the submit call that reached the stub, guarded because the
// stub answers on its own goroutine.
type submitRecorder struct {
	mu     sync.Mutex
	submit map[string]any
}

func (r *submitRecorder) body(t *testing.T) map[string]any {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.submit == nil {
		t.Fatal("no submit request reached the upstream")
	}

	return r.submit
}

// recordingNano answers a submit and then a completed poll, like nano-gpt would, keeping the
// submit body so a test can check what was sent.
func recordingNano(t *testing.T, videoURL string) (string, *submitRecorder) {
	t.Helper()

	recorder := &submitRecorder{}

	nano := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode submit body: %v", err)
			}

			recorder.mu.Lock()
			recorder.submit = body
			recorder.mu.Unlock()

			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"runId":"vid_1","status":"pending"}`))

			return
		}

		_, _ = w.Write([]byte(`{"data":{"status":"COMPLETED","output":{"video":{"url":"` + videoURL + `"}}}}`))
	})

	return nano, recorder
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
