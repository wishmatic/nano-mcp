package mcp

import (
	"context"
	"encoding/base64"
	"net/url"
	"path"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/filestore"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
	"github.com/wishmatic/nano-mcp/internal/resolve"
)

type generateVideoInput struct {
	Model          string `json:"model" jsonschema:"video model to generate with, such as veo2-video or kling_v2_1_std_5s; the names change, so a rejection means the running list is worth checking"`
	Prompt         string `json:"prompt" jsonschema:"what the video should show"`
	Image          string `json:"image,omitempty" jsonschema:"an image to animate, given as an http(s) URL, a data URI, raw base64, or a URL this server maps with IMAGE_URL_MAP; the bytes travel with the request, so nano-gpt needs to reach nothing. Naming one switches the call to image-to-video unless mode says otherwise"`
	NegativePrompt string `json:"negativePrompt,omitempty" jsonschema:"content to suppress; respected by Veo, Wan, Runway, Pixverse, and other models"`
	Duration       string `json:"duration,omitempty" jsonschema:"length in seconds as a string, such as 5 or 8; what a model accepts varies"`
	AspectRatio    string `json:"aspectRatio,omitempty" jsonschema:"16:9, 9:16, 1:1, 4:3, or 3:4 where the model supports it"`
	Resolution     string `json:"resolution,omitempty" jsonschema:"480p, 580p, 720p, 1080p, 2k, or 4k where the model supports it"`
	Mode           string `json:"mode,omitempty" jsonschema:"operation mode: text-to-video, image-to-video, reference-to-video, or video-edit"`
	Seed           *int   `json:"seed,omitempty" jsonschema:"seed, on the providers that honour one; it may improve reproducibility without guaranteeing it"`
	GenerateAudio  *bool  `json:"generateAudio,omitempty" jsonschema:"adds AI audio on Veo 3 and Lightricks models"`
	VideoURL       string `json:"videoUrl,omitempty" jsonschema:"public HTTPS source video to extend, edit, or upscale, for the models that take one; passed on as given"`
}

type generateVideoOutput struct {
	RunID       string  `json:"runId" jsonschema:"nano-gpt's id for the job"`
	Model       string  `json:"model" jsonschema:"the model that generated it"`
	VideoURL    string  `json:"videoUrl" jsonschema:"where this server serves the video; open it to watch the result"`
	ContentType string  `json:"contentType" jsonschema:"the media type the copy is served as"`
	Bytes       int64   `json:"bytes" jsonschema:"size of the hosted copy"`
	SourceURL   string  `json:"sourceUrl" jsonschema:"the provider URL the copy was taken from, which may expire"`
	CostUSD     float64 `json:"costUsd" jsonschema:"what nano-gpt billed the balance once the job finished"`
}

func registerGenerateVideo(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "generate_video",
		Description: "Generate a video through nano-gpt and wait for it, which takes minutes rather than " +
			"seconds. The call returns once the video is ready, as a resource link to a copy this server " +
			"hosts: MCP clients have no video content block, so a link is the only way back. Bills the " +
			"nano-gpt balance by duration and resolution.",
	}, h.generateVideo)
}

func (h *handlers) generateVideo(ctx context.Context, _ *mcp.CallToolRequest, in generateVideoInput) (
	*mcp.CallToolResult, generateVideoOutput, error,
) {
	req := nanogpt.VideoRequest{
		Model:          in.Model,
		Prompt:         in.Prompt,
		NegativePrompt: in.NegativePrompt,
		Duration:       in.Duration,
		AspectRatio:    in.AspectRatio,
		Resolution:     in.Resolution,
		Mode:           in.Mode,
		Seed:           in.Seed,
		GenerateAudio:  in.GenerateAudio,
		VideoURL:       in.VideoURL,
	}

	if in.Image != "" {
		image, err := h.resolver.Resolve(ctx, in.Image)
		if err != nil {
			return nil, generateVideoOutput{}, h.fail("generate_video", err)
		}

		req.ImageDataURL = imageDataURL(image)

		if req.Mode == "" {
			req.Mode = nanogpt.ModeImageToVideo
		}
	}

	video, err := h.nanogpt.GenerateVideo(ctx, req)
	if err != nil {
		return nil, generateVideoOutput{}, h.fail("generate_video", err)
	}

	stored, _, err := h.files.Fetch(ctx, video.URL)
	if err != nil {
		return nil, generateVideoOutput{}, h.fail("generate_video", err)
	}

	return videoLink(video, stored), newGenerateVideoOutput(video, stored), nil
}

// imageDataURL is nano-gpt's inline form for an image it cannot fetch itself.
func imageDataURL(image resolve.Image) string {
	return "data:" + image.MediaType + ";base64," + base64.StdEncoding.EncodeToString(image.Data)
}

// videoLink is the whole presentation of a generated video: MCP has no video content block, so
// the result points at the hosted copy rather than carrying the media.
func videoLink(video *nanogpt.Video, stored filestore.Asset) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ResourceLink{
		URI:         stored.URL,
		Name:        assetName(stored.URL),
		Title:       "Generated video",
		Description: "generated by " + video.Model,
		MIMEType:    stored.MediaType,
		Size:        &stored.Bytes,
	}}}
}

func assetName(assetURL string) string {
	parsed, err := url.Parse(assetURL)
	if err != nil {
		return "video"
	}

	return path.Base(parsed.Path)
}

func newGenerateVideoOutput(video *nanogpt.Video, stored filestore.Asset) generateVideoOutput {
	return generateVideoOutput{
		RunID:       video.RunID,
		Model:       video.Model,
		VideoURL:    stored.URL,
		ContentType: stored.MediaType,
		Bytes:       stored.Bytes,
		SourceURL:   video.URL,
		CostUSD:     video.CostUSD,
	}
}
