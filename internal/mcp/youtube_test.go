package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

const youtubeUpstreamBody = `{
	"transcripts": [
		{"url": "https://www.youtube.com/watch?v=one", "success": true, "title": "A Talk", "transcript": "hello\nthere"},
		{"url": "https://www.youtube.com/watch?v=two", "success": false, "error": "Video not found"}
	],
	"summary": {"requested": 2, "processed": 2, "successful": 1, "failed": 1, "totalCost": 0.01}
}`

const youtubeMissingBody = `{
	"transcripts": [{"url": "https://www.youtube.com/watch?v=missing", "success": false, "error": "Video not found"}],
	"summary": {"requested": 1, "processed": 1, "successful": 0, "failed": 1, "totalCost": 0}
}`

func youtubeUpstream(t *testing.T, body string) (string, *nanogpt.YouTubeRequest) {
	t.Helper()

	var got nanogpt.YouTubeRequest

	baseURL := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode upstream request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})

	return baseURL, &got
}

func TestYouTubeTranscribeReturnsTranscriptsAndTheCost(t *testing.T) {
	baseURL, got := youtubeUpstream(t, youtubeUpstreamBody)

	urls := []string{"https://www.youtube.com/watch?v=one", "https://www.youtube.com/watch?v=two"}

	result := callTool(t, nanoSession(t, baseURL), "youtube_transcribe", map[string]any{"urls": urls})
	if result.IsError {
		t.Fatalf("youtube_transcribe failed: %s", errorText(result))
	}

	if len(got.URLs) != 2 || got.URLs[0] != urls[0] {
		t.Errorf("upstream body = %+v, want the requested URLs", got.URLs)
	}

	var out youtubeTranscribeOutput
	structured(t, result, &out)

	if len(out.Transcripts) != 2 {
		t.Fatalf("transcripts = %+v, want one entry per URL", out.Transcripts)
	}

	if first := out.Transcripts[0]; !first.Success || first.Title != "A Talk" || first.Transcript != "hello\nthere" {
		t.Errorf("transcripts[0] = %+v, want the retrieved transcript", first)
	}

	want := transcriptSummary{Requested: 2, Processed: 2, Successful: 1, Failed: 1, TotalCost: 0.01}
	if out.Summary != want {
		t.Errorf("summary = %+v, want %+v", out.Summary, want)
	}
}

func TestYouTubeTranscribeKeepsPerURLFailuresAsResults(t *testing.T) {
	baseURL, _ := youtubeUpstream(t, youtubeMissingBody)

	args := map[string]any{"urls": []string{"https://www.youtube.com/watch?v=missing"}}

	result := callTool(t, nanoSession(t, baseURL), "youtube_transcribe", args)
	if result.IsError {
		t.Fatalf("IsError = true, want a failing URL reported inside the result: %s", errorText(result))
	}

	var out youtubeTranscribeOutput
	structured(t, result, &out)

	if len(out.Transcripts) != 1 {
		t.Fatalf("transcripts = %+v, want one entry", out.Transcripts)
	}

	if out.Transcripts[0].Success || !strings.Contains(out.Transcripts[0].Error, "not found") {
		t.Errorf("transcripts[0] = %+v, want nano-gpt's reason for the failure", out.Transcripts[0])
	}

	if out.Summary.TotalCost != 0 {
		t.Errorf("totalCost = %v, want a failed transcript to be free", out.Summary.TotalCost)
	}
}

func TestYouTubeTranscribeRejectsMoreThanTheCap(t *testing.T) {
	var calls atomic.Int64

	baseURL := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
	})

	tooMany := make([]string, 0, nanogpt.MaxTranscribeURLs+1)
	for i := 0; i <= nanogpt.MaxTranscribeURLs; i++ {
		tooMany = append(tooMany, "https://www.youtube.com/watch?v=video")
	}

	result := callTool(t, nanoSession(t, baseURL), "youtube_transcribe", map[string]any{"urls": tooMany})
	if !result.IsError {
		t.Fatal("IsError = false, want the URL count rejected")
	}

	if text := errorText(result); !strings.Contains(text, "1 to 10") {
		t.Errorf("error text = %q, want it to name the range", text)
	}

	if calls.Load() != 0 {
		t.Errorf("upstream calls = %d, want none for a rejected URL count", calls.Load())
	}
}

func TestYouTubeTranscribeReportsAnEmptyBalance(t *testing.T) {
	baseURL := upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"error":"Insufficient balance. Current balance: $0.50, required: $1.00"}`))
	})

	args := map[string]any{"urls": []string{"https://www.youtube.com/watch?v=one"}}

	result := callTool(t, nanoSession(t, baseURL), "youtube_transcribe", args)
	if !result.IsError {
		t.Fatal("IsError = false, want the upstream 402 to fail the call")
	}

	if text := errorText(result); !strings.Contains(text, "Insufficient balance") {
		t.Errorf("error text = %q, want the balance problem named", text)
	}
}
