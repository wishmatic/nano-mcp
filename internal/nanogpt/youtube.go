package nanogpt

import (
	"context"
	"fmt"
	"time"
)

const (
	youtubePath = "/api/youtube-transcribe"

	// MaxTranscribeURLs is the endpoint's documented cap.
	MaxTranscribeURLs = 10

	// transcribeBase covers the request around the caption fetches and transcribePerURL
	// bounds each one, so a single URL fails in 45s rather than waiting out a ceiling
	// sized for ten.
	transcribeBase   = 30 * time.Second
	transcribePerURL = 15 * time.Second
)

type YouTubeRequest struct {
	URLs []string `json:"urls"`
}

type YouTubeTranscripts struct {
	Transcripts []YouTubeTranscript `json:"transcripts"`
	Summary     YouTubeSummary      `json:"summary"`
}

type YouTubeTranscript struct {
	URL        string `json:"url"`
	Success    bool   `json:"success"`
	Title      string `json:"title,omitempty"`
	Transcript string `json:"transcript,omitempty"`
	Error      string `json:"error,omitempty"`
}

type YouTubeSummary struct {
	Requested  int     `json:"requested"`
	Processed  int     `json:"processed"`
	Successful int     `json:"successful"`
	Failed     int     `json:"failed"`
	TotalCost  float64 `json:"totalCost"`
}

func (c *Client) TranscribeYouTube(ctx context.Context, urls []string) (*YouTubeTranscripts, error) {
	if len(urls) == 0 || len(urls) > MaxTranscribeURLs {
		return nil, fmt.Errorf("youtube_transcribe takes 1 to %d URLs, got %d", MaxTranscribeURLs, len(urls))
	}

	var out YouTubeTranscripts

	cred := credential{header: "x-api-key", value: c.apiKey}
	if err := c.post(ctx, youtubePath, transcribeBudget(len(urls)), cred, YouTubeRequest{URLs: urls}, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

func transcribeBudget(urls int) time.Duration {
	return transcribeBase + time.Duration(urls)*transcribePerURL
}
