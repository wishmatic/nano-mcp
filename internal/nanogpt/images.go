package nanogpt

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

const (
	imagesPath = "/api/v1/images/generations"

	// ImageGenerationBudget bounds one synchronous generation. The endpoint holds the connection
	// until the image exists, which the documented models take well past FastTimeout for.
	ImageGenerationBudget = 5 * time.Minute

	// ResponseFormatBase64 asks for the bytes inline. It is nano-gpt's own default, and is named
	// because the alternative is a signed URL that expires within the hour.
	ResponseFormatBase64 = "b64_json"
)

// ImageRequest is the part of nano-gpt's OpenAI-compatible image request this server exposes; the
// rest of the documented fields are knobs a single model each.
//
// Pointers keep an unset field distinguishable from an explicit zero, which several of these
// accept as a value.
type ImageRequest struct {
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt"`
	N      *int   `json:"n,omitempty"`
	Size   string `json:"size,omitempty"`

	ResponseFormat string `json:"response_format,omitempty"`
	User           string `json:"user,omitempty"`

	// nano-gpt takes its inputs as data URLs rather than addresses it would have to fetch.
	ImageDataURL  string   `json:"imageDataUrl,omitempty"`
	ImageDataURLs []string `json:"imageDataUrls,omitempty"`
	MaskDataURL   string   `json:"maskDataUrl,omitempty"`

	Strength          *float64 `json:"strength,omitempty"`
	GuidanceScale     *float64 `json:"guidance_scale,omitempty"`
	NumInferenceSteps *int     `json:"num_inference_steps,omitempty"`
	Seed              *int     `json:"seed,omitempty"`
}

type imageResponse struct {
	Created int         `json:"created"`
	Data    []imageData `json:"data"`
	Cost    float64     `json:"cost"`
}

type imageData struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json"`
}

// GeneratedImage is one image nano-gpt produced: the bytes when it answered inline, or the signed
// URL it falls back to when its own upload failed. It carries one or the other, never both.
type GeneratedImage struct {
	Data []byte
	URL  string
}

type ImageResult struct {
	Images  []GeneratedImage
	CostUSD float64
}

// GenerateImages posts one prompt and returns what came back. The endpoint is synchronous, so a
// caller gets finished images or an error, with no run id in between.
func (c *Client) GenerateImages(ctx context.Context, req ImageRequest) (*ImageResult, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}

	var out imageResponse

	cred := credential{header: "Authorization", value: "Bearer " + c.apiKey}
	if err := c.post(ctx, imagesPath, ImageGenerationBudget, cred, req, &out); err != nil {
		return nil, err
	}

	return out.result()
}

func (r imageResponse) result() (*ImageResult, error) {
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("%s answered with no images", imagesPath)
	}

	images := make([]GeneratedImage, 0, len(r.Data))

	for index, entry := range r.Data {
		image, err := entry.image()
		if err != nil {
			return nil, fmt.Errorf("image %d of %d: %w", index+1, len(r.Data), err)
		}

		images = append(images, image)
	}

	return &ImageResult{Images: images, CostUSD: r.Cost}, nil
}

func (d imageData) image() (GeneratedImage, error) {
	switch {
	case d.B64JSON != "":
		data, err := decodeImageData(d.B64JSON)
		if err != nil {
			return GeneratedImage{}, err
		}

		return GeneratedImage{Data: data}, nil
	case d.URL != "":
		return GeneratedImage{URL: d.URL}, nil
	default:
		return GeneratedImage{}, fmt.Errorf("carries neither b64_json nor url")
	}
}

// decodeImageData accepts the data URI prefix some routes wrap these bytes in, and base64 without
// its padding, both of which turn up for this field.
func decodeImageData(encoded string) ([]byte, error) {
	if strings.HasPrefix(encoded, "data:") {
		_, payload, ok := strings.Cut(encoded, ",")
		if !ok {
			return nil, fmt.Errorf("b64_json starts a data URI it does not finish")
		}

		encoded = payload
	}

	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding} {
		data, err := encoding.DecodeString(strings.TrimSpace(encoded))
		if err == nil {
			return data, nil
		}
	}

	return nil, fmt.Errorf("b64_json is not base64")
}

func (r ImageRequest) validate() error {
	if strings.TrimSpace(r.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}

	return nil
}
