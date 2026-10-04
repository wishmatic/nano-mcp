package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/filestore"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

// A generated image is marked as being for the user and the assistant alike, which is what lets a
// vision-capable model see it rather than only the URL it is served from.
const (
	roleUser      mcp.Role = "user"
	roleAssistant mcp.Role = "assistant"
)

type generateImageInput struct {
	Prompt string `json:"prompt" jsonschema:"what the image should show"`
	Model  string `json:"model,omitempty" jsonschema:"image model id, such as nano-banana or hidream; the names change, so a rejection means nano-gpt's model list is worth checking. Unset uses nano-gpt's own default"`
	N      *int   `json:"n,omitempty" jsonschema:"how many images to generate, each of them billed; unset uses nano-gpt's own default of 1"`
	Size   string `json:"size,omitempty" jsonschema:"requested output size or a model-specific resolution, such as 1024x1024, 1376x768, or auto; what a model accepts varies"`

	Image  string   `json:"image,omitempty" jsonschema:"an image to transform instead of generating from scratch, given as an http(s) URL, a data URI, raw base64, or a URL this server maps with IMAGE_URL_MAP; the bytes travel with the request, so nano-gpt needs to reach nothing"`
	Images []string `json:"images,omitempty" jsonschema:"input images for the models that take more than one, each resolved the way image is"`
	Mask   string   `json:"mask,omitempty" jsonschema:"an inpainting mask, resolved the way image is; white marks the regions to generate"`

	Seed              *int     `json:"seed,omitempty" jsonschema:"seed, on the models that take one; it may improve reproducibility without guaranteeing it"`
	Strength          *float64 `json:"strength,omitempty" jsonschema:"how far the output may move from the input image, from 0 to 1, where lower stays closer to it"`
	GuidanceScale     *float64 `json:"guidanceScale,omitempty" jsonschema:"how closely the model follows the prompt, from 0 to 20"`
	NumInferenceSteps *int     `json:"numInferenceSteps,omitempty" jsonschema:"denoising steps, from 1 to 100, where more is slower and usually better"`
}

type generateImageOutput struct {
	Model   string   `json:"model" jsonschema:"the model that was asked for, empty when nano-gpt's own default was used"`
	Count   int      `json:"count" jsonschema:"number of images generated"`
	URLs    []string `json:"urls" jsonschema:"where this server serves each image, which is also attached to this result"`
	CostUSD float64  `json:"costUsd" jsonschema:"what nano-gpt billed the balance for the call"`
}

func registerGenerateImage(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "generate_image",
		Description: "Generate one or more images through nano-gpt and wait for them, which takes seconds to a " +
			"few minutes. Every image is attached to the result, with the URL this server serves its copy from " +
			"next to it. Passing image transforms that image instead of generating from scratch, and mask " +
			"confines the change to the regions it marks. Bills the nano-gpt balance for each image; the charge " +
			"is reported in costUsd.",
	}, h.generateImage)
}

func (h *handlers) generateImage(ctx context.Context, _ *mcp.CallToolRequest, in generateImageInput) (
	*mcp.CallToolResult, generateImageOutput, error,
) {
	req, err := h.imageRequest(ctx, in)
	if err != nil {
		return nil, generateImageOutput{}, h.fail("generate_image", err)
	}

	result, err := h.nanogpt.GenerateImages(ctx, req)
	if err != nil {
		return nil, generateImageOutput{}, h.fail("generate_image", err)
	}

	images, err := h.storeImages(ctx, result.Images)
	if err != nil {
		return nil, generateImageOutput{}, h.fail("generate_image", err)
	}

	return imageContent(images), newGenerateImageOutput(in.Model, result, images), nil
}

// imageRequest carries the caller's addresses into the data URLs nano-gpt takes, so that the image
// it transforms is one this deployment can read rather than only one nano-gpt could.
func (h *handlers) imageRequest(ctx context.Context, in generateImageInput) (nanogpt.ImageRequest, error) {
	image, err := h.resolveDataURL(ctx, in.Image)
	if err != nil {
		return nanogpt.ImageRequest{}, err
	}

	mask, err := h.resolveDataURL(ctx, in.Mask)
	if err != nil {
		return nanogpt.ImageRequest{}, err
	}

	images, err := h.resolveDataURLs(ctx, in.Images)
	if err != nil {
		return nanogpt.ImageRequest{}, err
	}

	return nanogpt.ImageRequest{
		Model:  in.Model,
		Prompt: in.Prompt,
		N:      in.N,
		Size:   in.Size,

		ResponseFormat:    nanogpt.ResponseFormatBase64,
		Strength:          in.Strength,
		GuidanceScale:     in.GuidanceScale,
		NumInferenceSteps: in.NumInferenceSteps,
		Seed:              in.Seed,

		ImageDataURL:  image,
		ImageDataURLs: images,
		MaskDataURL:   mask,
	}, nil
}

func (h *handlers) resolveDataURLs(ctx context.Context, addresses []string) ([]string, error) {
	if len(addresses) == 0 {
		return nil, nil
	}

	dataURLs := make([]string, 0, len(addresses))

	for _, address := range addresses {
		dataURL, err := h.resolveDataURL(ctx, address)
		if err != nil {
			return nil, err
		}

		dataURLs = append(dataURLs, dataURL)
	}

	return dataURLs, nil
}

// resolveDataURL reads an address into the inline form nano-gpt takes; an address the caller left
// out resolves to none.
func (h *handlers) resolveDataURL(ctx context.Context, address string) (string, error) {
	if address == "" {
		return "", nil
	}

	image, err := h.resolver.Resolve(ctx, address)
	if err != nil {
		return "", err
	}

	return imageDataURL(image), nil
}

// storedImage is a generated image where it is served from, and the bytes it is served as.
type storedImage struct {
	asset filestore.Asset
	data  []byte
}

func (h *handlers) storeImages(ctx context.Context, images []nanogpt.GeneratedImage) ([]storedImage, error) {
	stored := make([]storedImage, 0, len(images))

	for _, image := range images {
		one, err := h.storeImage(ctx, image)
		if err != nil {
			return nil, err
		}

		stored = append(stored, one)
	}

	return stored, nil
}

// storeImage keeps a copy of one generated image. nano-gpt answers inline when asked to, and falls
// back to a signed URL when its own upload fails; either way what is kept here is the copy that
// outlives the call.
func (h *handlers) storeImage(ctx context.Context, image nanogpt.GeneratedImage) (storedImage, error) {
	if image.URL != "" {
		asset, data, err := h.files.Fetch(ctx, image.URL)
		if err != nil {
			return storedImage{}, err
		}

		return storedImage{asset: asset, data: data}, nil
	}

	mediaType, ok := filestore.DetectMediaType(image.Data)
	if !ok {
		return storedImage{}, fmt.Errorf("the generated image is not a type this server serves")
	}

	asset, err := h.files.Put(ctx, image.Data, mediaType)
	if err != nil {
		return storedImage{}, err
	}

	return storedImage{asset: asset, data: image.Data}, nil
}

// imageContent is the whole presentation of a generated image: the URL this server serves the copy
// from as text, immediately followed by the image itself. MCP has an image content block, so unlike
// a generated video the media travels with the call.
func imageContent(images []storedImage) *mcp.CallToolResult {
	content := make([]mcp.Content, 0, 2*len(images))

	for _, image := range images {
		content = append(content, &mcp.TextContent{Text: image.asset.URL}, imageBlock(image))
	}

	return &mcp.CallToolResult{Content: content}
}

func imageBlock(image storedImage) *mcp.ImageContent {
	return &mcp.ImageContent{
		Data:        image.data,
		MIMEType:    image.asset.MediaType,
		Annotations: &mcp.Annotations{Audience: []mcp.Role{roleUser, roleAssistant}},
	}
}

func newGenerateImageOutput(model string, result *nanogpt.ImageResult, images []storedImage) generateImageOutput {
	urls := make([]string, 0, len(images))

	for _, image := range images {
		urls = append(urls, image.asset.URL)
	}

	return generateImageOutput{
		Model:   model,
		Count:   len(urls),
		URLs:    urls,
		CostUSD: result.CostUSD,
	}
}
