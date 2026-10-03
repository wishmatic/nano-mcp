package nanogpt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

const youtubeResponseBody = `{
	"transcripts": [
		{"url": "https://www.youtube.com/watch?v=one", "success": true, "title": "A Talk", "transcript": "hello\nthere"},
		{"url": "https://www.youtube.com/watch?v=two", "success": false, "error": "Video not found or transcripts not available"}
	],
	"summary": {"requested": 2, "processed": 2, "successful": 1, "failed": 1, "totalCost": 0.01}
}`

func TestTranscribeYouTubeSendsTheApiKeyHeader(t *testing.T) {
	srv, rec := recordingServer(t, youtubeResponseBody, http.StatusOK)

	urls := []string{"https://www.youtube.com/watch?v=one", "https://www.youtube.com/watch?v=two"}
	if _, err := newTestClient(t, srv.URL).TranscribeYouTube(context.Background(), urls); err != nil {
		t.Fatalf("TranscribeYouTube() error: %v", err)
	}

	rec.assertHeader(t, "Content-Type", "application/json")
	rec.assertHeader(t, "x-api-key", testKey)
	rec.assertHeader(t, "Authorization", "")

	if rec.method != http.MethodPost {
		t.Errorf("method = %s, want POST", rec.method)
	}

	if rec.path != "/api/youtube-transcribe" {
		t.Errorf("path = %s, want /api/youtube-transcribe", rec.path)
	}

	got, ok := rec.body["urls"].([]any)
	if !ok || len(got) != len(urls) {
		t.Fatalf("urls = %#v, want the requested URLs", rec.body["urls"])
	}

	for i, want := range urls {
		if got[i] != want {
			t.Errorf("urls[%d] = %v, want %s", i, got[i], want)
		}
	}
}

func TestTranscribeYouTubeDecodesTheResponse(t *testing.T) {
	srv, _ := recordingServer(t, youtubeResponseBody, http.StatusOK)

	got, err := newTestClient(t, srv.URL).TranscribeYouTube(
		context.Background(),
		[]string{"https://www.youtube.com/watch?v=one", "https://www.youtube.com/watch?v=two"},
	)
	if err != nil {
		t.Fatalf("TranscribeYouTube() error: %v", err)
	}

	if len(got.Transcripts) != 2 {
		t.Fatalf("transcripts = %+v, want one entry per URL", got.Transcripts)
	}

	first := got.Transcripts[0]
	if !first.Success || first.Title != "A Talk" || first.Transcript != "hello\nthere" {
		t.Errorf("transcripts[0] = %+v, want the retrieved transcript", first)
	}

	second := got.Transcripts[1]
	if second.Success || !strings.Contains(second.Error, "not available") {
		t.Errorf("transcripts[1] = %+v, want the per-URL failure", second)
	}

	want := YouTubeSummary{Requested: 2, Processed: 2, Successful: 1, Failed: 1, TotalCost: 0.01}
	if got.Summary != want {
		t.Errorf("summary = %+v, want %+v", got.Summary, want)
	}
}

func TestTranscribeYouTubeRejectsURLCountsOutsideTheCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a request was sent for a URL count the endpoint does not accept")
	}))
	t.Cleanup(srv.Close)

	cases := map[string][]string{
		"none":   {},
		"eleven": {"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"},
	}

	for name, urls := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := newTestClient(t, srv.URL).TranscribeYouTube(context.Background(), urls)
			if err == nil || !strings.Contains(err.Error(), "1 to 10") {
				t.Errorf("TranscribeYouTube() error = %v, want the 1 to 10 range named", err)
			}
		})
	}
}

func TestTranscribeYouTubeReportsUpstreamFailures(t *testing.T) {
	statuses := map[int]string{
		http.StatusBadRequest:      "Please provide an array of YouTube URLs",
		http.StatusUnauthorized:    "Invalid session",
		http.StatusPaymentRequired: "Insufficient balance",
		http.StatusTooManyRequests: "Rate limit exceeded",
	}

	for status, body := range statuses {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, _ := recordingServer(t, `{"error":"`+body+`"}`, status)

			_, err := newTestClient(t, srv.URL).TranscribeYouTube(
				context.Background(),
				[]string{"https://www.youtube.com/watch?v=one"},
			)
			if err == nil || !strings.Contains(err.Error(), strconv.Itoa(status)) {
				t.Errorf("TranscribeYouTube() error = %v, want it to name the status", err)
			}

			if err != nil && !strings.Contains(err.Error(), body) {
				t.Errorf("error = %q, want the body quoted", err)
			}
		})
	}
}

func TestTranscribeYouTubeRejectsAMalformedResponse(t *testing.T) {
	srv, _ := recordingServer(t, `{"transcripts": `, http.StatusOK)

	_, err := newTestClient(t, srv.URL).TranscribeYouTube(
		context.Background(),
		[]string{"https://www.youtube.com/watch?v=one"},
	)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("TranscribeYouTube() error = %v, want a decode error", err)
	}
}

func TestTranscribeBudgetStaysWithinTheCap(t *testing.T) {
	atCap := transcribeBudget(MaxTranscribeURLs)

	if atCap > MaxCallBudget {
		t.Errorf("budget at %d URLs = %s, want it within MaxCallBudget (%s)", MaxTranscribeURLs, atCap, MaxCallBudget)
	}

	if single := transcribeBudget(1); single >= atCap {
		t.Errorf("budget for one URL = %s, want it below the full request's %s", single, atCap)
	}
}
