package nanogpt

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGenerateVideoSubmitsTheExposedFields(t *testing.T) {
	srv, calls := videoUpstream(t, `{"runId":"vid_1","status":"pending"}`, completedStatus("https://cdn.example.com/a.mp4"))

	seed := 7
	audio := true

	video, err := videoTestClient(t, srv.URL).GenerateVideo(context.Background(), VideoRequest{
		Model:          "veo2-video",
		Prompt:         "a lake at sunset",
		NegativePrompt: "blur",
		Duration:       "8",
		AspectRatio:    "16:9",
		Resolution:     "1080p",
		Mode:           "text-to-video",
		Seed:           &seed,
		GenerateAudio:  &audio,
		ImageURL:       "https://images.example.com/start.png",
		VideoURL:       "https://videos.example.com/source.mp4",
	})
	if err != nil {
		t.Fatalf("GenerateVideo() error: %v", err)
	}

	if video.URL != "https://cdn.example.com/a.mp4" || video.RunID != "vid_1" {
		t.Errorf("video = %+v, want the run id and the completed URL", video)
	}

	if video.Model != "veo2-video" {
		t.Errorf("Model = %q, want the requested model back when the submit response omits it", video.Model)
	}

	post := calls.submit(t)
	post.assertHeader(t, "x-api-key", testKey)

	want := map[string]any{
		"model":           "veo2-video",
		"prompt":          "a lake at sunset",
		"negative_prompt": "blur",
		"duration":        "8",
		"aspect_ratio":    "16:9",
		"resolution":      "1080p",
		"mode":            "text-to-video",
		"seed":            float64(7),
		"generateAudio":   true,
		"imageUrl":        "https://images.example.com/start.png",
		"videoUrl":        "https://videos.example.com/source.mp4",
	}

	for key, value := range want {
		post.assertBody(t, key, value)
	}
}

func TestGenerateVideoOmitsUnsetFields(t *testing.T) {
	srv, calls := videoUpstream(t, `{"runId":"vid_1"}`, completedStatus("https://cdn.example.com/a.mp4"))

	_, err := videoTestClient(t, srv.URL).GenerateVideo(context.Background(), VideoRequest{
		Model:  "veo2-video",
		Prompt: "a lake",
	})
	if err != nil {
		t.Fatalf("GenerateVideo() error: %v", err)
	}

	post := calls.submit(t)

	for _, key := range []string{"seed", "duration", "imageUrl", "videoUrl", "negative_prompt"} {
		post.assertBodyOmits(t, key)
	}
}

func TestGenerateVideoPollsUntilTheJobIsDone(t *testing.T) {
	srv, calls := videoUpstream(t, `{"runId":"vid_1","status":"pending"}`,
		`{"data":{"status":"IN_QUEUE","requestId":"vid_1"}}`,
		`{"data":{"status":"IN_PROGRESS"}}`,
		completedStatus("https://cdn.example.com/a.mp4"),
	)

	video, err := videoTestClient(t, srv.URL).GenerateVideo(context.Background(), VideoRequest{
		Model:  "veo2-video",
		Prompt: "a lake",
	})
	if err != nil {
		t.Fatalf("GenerateVideo() error: %v", err)
	}

	if video.URL != "https://cdn.example.com/a.mp4" {
		t.Errorf("URL = %q, want the completed video", video.URL)
	}

	polls, query := calls.polling(t)
	if polls != 3 {
		t.Errorf("polls = %d, want one per queued and processing status plus the completed one", polls)
	}

	if got := query.Get("requestId"); got != "vid_1" {
		t.Errorf("requestId = %q, want vid_1", got)
	}
}

func TestGenerateVideoReportsTheCost(t *testing.T) {
	srv, _ := videoUpstream(t, `{"runId":"vid_1"}`, `{"data":{"status":"COMPLETED","cost":0.35,
		"output":{"video":{"url":"https://cdn.example.com/a.mp4"}}}}`)

	video, err := videoTestClient(t, srv.URL).GenerateVideo(context.Background(), VideoRequest{
		Model:  "veo2-video",
		Prompt: "a lake",
	})
	if err != nil {
		t.Fatalf("GenerateVideo() error: %v", err)
	}

	if video.CostUSD != 0.35 {
		t.Errorf("CostUSD = %v, want the charged 0.35", video.CostUSD)
	}
}

func TestGenerateVideoReadsTheFlatStatusShape(t *testing.T) {
	srv, _ := videoUpstream(t, `{"runId":"vid_2"}`,
		`{"requestId":"vid_2","status":"completed","videoUrl":"https://cdn.example.com/b.webm"}`,
	)

	video, err := videoTestClient(t, srv.URL).GenerateVideo(context.Background(), VideoRequest{
		Model:  "veo2-video",
		Prompt: "a lake",
	})
	if err != nil {
		t.Fatalf("GenerateVideo() error: %v", err)
	}

	if video.URL != "https://cdn.example.com/b.webm" {
		t.Errorf("URL = %q, want the URL from the flat response shape", video.URL)
	}
}

func TestGenerateVideoSurfacesAFailedJob(t *testing.T) {
	tests := map[string]struct {
		status string
		want   string
	}{
		"failed": {
			status: `{"data":{"status":"FAILED","error":"Content policy violation",
				"userFriendlyError":"please reword the prompt"}}`,
			want: "Content policy violation",
		},
		"canceled": {
			status: `{"data":{"status":"CANCELED","error":"canceled by the provider"}}`,
			want:   "canceled by the provider",
		},
		"completed without a URL": {
			status: `{"data":{"status":"COMPLETED"}}`,
			want:   "without a URL",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv, _ := videoUpstream(t, `{"runId":"vid_3"}`, tt.status)

			_, err := videoTestClient(t, srv.URL).GenerateVideo(context.Background(), VideoRequest{
				Model:  "veo2-video",
				Prompt: "a lake",
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("GenerateVideo() error = %v, want it to carry %q", err, tt.want)
			}

			if !strings.Contains(err.Error(), "vid_3") {
				t.Errorf("error = %v, want the run id named so the job can be followed up", err)
			}
		})
	}
}

func TestGenerateVideoGivesUpAfterItsBudget(t *testing.T) {
	srv, _ := videoUpstream(t, `{"runId":"vid_4"}`, `{"data":{"status":"IN_PROGRESS"}}`)

	c := videoTestClient(t, srv.URL)
	c.videoWaitBudget = 30 * time.Millisecond

	_, err := c.GenerateVideo(context.Background(), VideoRequest{Model: "veo2-video", Prompt: "a lake"})
	if err == nil || !strings.Contains(err.Error(), "still processing") {
		t.Fatalf("GenerateVideo() error = %v, want a timeout naming the last status", err)
	}

	if !strings.Contains(err.Error(), "vid_4") {
		t.Errorf("error = %v, want the run id named", err)
	}
}

func TestGenerateVideoRejectsAnEmptyModelOrPrompt(t *testing.T) {
	tests := map[string]VideoRequest{
		"no model":     {Prompt: "a lake"},
		"no prompt":    {Model: "veo2-video"},
		"blank prompt": {Model: "veo2-video", Prompt: "   "},
	}

	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newTestClient(t, freeServer(t)).GenerateVideo(context.Background(), req)
			if err == nil || !strings.Contains(err.Error(), "required") {
				t.Errorf("GenerateVideo() error = %v, want it to name the missing field", err)
			}
		})
	}
}

func TestGenerateVideoRejectsASubmitWithoutARunID(t *testing.T) {
	srv, _ := videoUpstream(t, `{"status":"pending"}`)

	_, err := videoTestClient(t, srv.URL).GenerateVideo(context.Background(), VideoRequest{
		Model: "veo2-video", Prompt: "a lake",
	})
	if err == nil || !strings.Contains(err.Error(), "runId") {
		t.Errorf("GenerateVideo() error = %v, want it to name the missing runId", err)
	}
}

func TestGenerateVideoSurfacesStatusFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"slow down"}`))

			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"runId":"vid_5"}`))
	}))
	t.Cleanup(srv.Close)

	_, err := videoTestClient(t, srv.URL).GenerateVideo(context.Background(), VideoRequest{
		Model: "veo2-video", Prompt: "a lake",
	})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("GenerateVideo() error = %v, want the polling status surfaced", err)
	}
}

func completedStatus(videoURL string) string {
	return `{"data":{"status":"COMPLETED","cost":0.35,"output":{"video":{"url":"` + videoURL + `"}}}}`
}

// videoTestClient shortens the poll loop so a test drives a whole generation without waiting
// out nano-gpt's own cadence.
func videoTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()

	c := newTestClient(t, baseURL)
	c.videoPollInterval = time.Millisecond
	c.videoWaitBudget = 2 * time.Second

	return c
}

func freeServer(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("the upstream was called with %s %s", r.Method, r.URL.Path)
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

type videoCalls struct {
	mu    sync.Mutex
	post  *recordedRequest
	polls int
	query url.Values
}

func (c *videoCalls) record(t *testing.T, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if r.Method == http.MethodPost {
		req := &recordedRequest{
			method:  r.Method,
			path:    r.URL.Path,
			headers: r.Header.Clone(),
			body:    map[string]any{},
		}

		if err := json.NewDecoder(r.Body).Decode(&req.body); err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("decode submit body: %v", err)
		}

		c.post = req

		return
	}

	c.polls++
	c.query = r.URL.Query()
}

func (c *videoCalls) submit(t *testing.T) *recordedRequest {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.post == nil {
		t.Fatal("no submit request reached the upstream")

		return nil
	}

	if c.post.path != videoPath {
		t.Errorf("submit path = %s, want %s", c.post.path, videoPath)
	}

	return c.post
}

func (c *videoCalls) polling(t *testing.T) (int, url.Values) {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.polls, c.query
}

// videoUpstream answers one submit and then the queued status bodies in order, repeating the
// last of them once the queue runs out, so a stalled job stays stalled until the caller gives up.
func videoUpstream(t *testing.T, submit string, statuses ...string) (*httptest.Server, *videoCalls) {
	t.Helper()

	calls := &videoCalls{}
	queue := slices.Clone(statuses)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.record(t, r)

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == videoPath:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(submit))
		case r.Method == http.MethodGet && r.URL.Path == videoStatusPath:
			body := `{}`
			if len(queue) > 1 {
				body, queue = queue[0], queue[1:]
			} else if len(queue) == 1 {
				body = queue[0]
			}

			_, _ = w.Write([]byte(body))
		default:
			t.Errorf("unexpected upstream call %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	return srv, calls
}
