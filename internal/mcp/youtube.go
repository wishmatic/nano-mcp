package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

type youtubeTranscribeInput struct {
	URLs []string `json:"urls" jsonschema:"1 to 10 YouTube video URLs to transcribe"`
}

type youtubeTranscribeOutput struct {
	Transcripts []transcriptEntry `json:"transcripts" jsonschema:"one entry per requested URL, in the order they were asked for"`
	Summary     transcriptSummary `json:"summary" jsonschema:"what the request cost and how many URLs succeeded"`
}

type transcriptEntry struct {
	URL        string `json:"url" jsonschema:"video that was transcribed"`
	Success    bool   `json:"success" jsonschema:"whether the transcript was retrieved"`
	Title      string `json:"title,omitempty" jsonschema:"video title when nano-gpt reported one"`
	Transcript string `json:"transcript,omitempty" jsonschema:"the transcript, one line per caption cue"`
	Error      string `json:"error,omitempty" jsonschema:"why the transcript is missing, when it is"`
}

type transcriptSummary struct {
	Requested  int     `json:"requested" jsonschema:"URLs asked for"`
	Processed  int     `json:"processed" jsonschema:"URLs nano-gpt attempted"`
	Successful int     `json:"successful" jsonschema:"transcripts retrieved"`
	Failed     int     `json:"failed" jsonschema:"transcripts that could not be retrieved"`
	TotalCost  float64 `json:"totalCost" jsonschema:"what nano-gpt charged in USD; failed URLs are free"`
}

func registerYouTubeTranscribe(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "youtube_transcribe",
		Description: "Fetch the transcript of up to 10 YouTube videos through nano-gpt, at about a cent per video. " +
			"Only successful transcripts are charged, and a video without captions comes back as a failed entry " +
			"rather than an error. nano-gpt allows 10 of these requests a minute.",
	}, h.youtubeTranscribe)
}

func (h *handlers) youtubeTranscribe(ctx context.Context, _ *mcp.CallToolRequest, in youtubeTranscribeInput) (
	*mcp.CallToolResult, youtubeTranscribeOutput, error,
) {
	got, err := h.nanogpt.TranscribeYouTube(ctx, in.URLs)
	if err != nil {
		return nil, youtubeTranscribeOutput{}, h.fail("youtube_transcribe", err)
	}

	return nil, newYouTubeTranscribeOutput(got), nil
}

func newYouTubeTranscribeOutput(got *nanogpt.YouTubeTranscripts) youtubeTranscribeOutput {
	entries := make([]transcriptEntry, 0, len(got.Transcripts))
	for _, transcript := range got.Transcripts {
		entries = append(entries, transcriptEntry{
			URL:        transcript.URL,
			Success:    transcript.Success,
			Title:      transcript.Title,
			Transcript: transcript.Transcript,
			Error:      transcript.Error,
		})
	}

	return youtubeTranscribeOutput{
		Transcripts: entries,
		Summary: transcriptSummary{
			Requested:  got.Summary.Requested,
			Processed:  got.Summary.Processed,
			Successful: got.Summary.Successful,
			Failed:     got.Summary.Failed,
			TotalCost:  got.Summary.TotalCost,
		},
	}
}
