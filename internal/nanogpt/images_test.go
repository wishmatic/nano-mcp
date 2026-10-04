package nanogpt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

const testImageB64 = "cG5nIGJ5dGVz"

func TestGenerateImagesSendsTheExposedFields(t *testing.T) {
	body := `{"created":1,"data":[{"b64_json":"` + testImageB64 + `"}],"cost":0.04}`

	srv, post := recordingServer(t, body, http.StatusOK)

	count := 3
	strength := 0.6
	guidance := 7.5
	steps := 30
	seed := 42

	result, err := newTestClient(t, srv.URL).GenerateImages(context.Background(), ImageRequest{
		Model:             "nano-banana",
		Prompt:            "a lake at sunset",
		N:                 &count,
		Size:              "1024x1024",
		ResponseFormat:    ResponseFormatBase64,
		ImageDataURL:      "data:image/png;base64,cG5n",
		ImageDataURLs:     []string{"data:image/png;base64,cG5n", "data:image/jpeg;base64,anBn"},
		MaskDataURL:       "data:image/png;base64,bWFzaw==",
		Strength:          &strength,
		GuidanceScale:     &guidance,
		NumInferenceSteps: &steps,
		Seed:              &seed,
	})
	if err != nil {
		t.Fatalf("GenerateImages() error: %v", err)
	}

	if post.method != http.MethodPost || post.path != imagesPath {
		t.Errorf("call = %s %s, want POST %s", post.method, post.path, imagesPath)
	}

	post.assertHeader(t, "Authorization", "Bearer "+testKey)

	post.assertBody(t, "model", "nano-banana")
	post.assertBody(t, "prompt", "a lake at sunset")
	post.assertBodyNumber(t, "n", 3)
	post.assertBody(t, "size", "1024x1024")
	post.assertBody(t, "response_format", ResponseFormatBase64)
	post.assertBody(t, "imageDataUrl", "data:image/png;base64,cG5n")
	post.assertBody(t, "maskDataUrl", "data:image/png;base64,bWFzaw==")
	post.assertBodyNumber(t, "strength", 0.6)
	post.assertBodyNumber(t, "guidance_scale", 7.5)
	post.assertBodyNumber(t, "num_inference_steps", 30)
	post.assertBodyNumber(t, "seed", 42)

	want := []any{"data:image/png;base64,cG5n", "data:image/jpeg;base64,anBn"}

	if urls, ok := post.body["imageDataUrls"].([]any); !ok || !slices.Equal(urls, want) {
		t.Errorf("imageDataUrls = %v, want %v", post.body["imageDataUrls"], want)
	}

	if len(result.Images) != 1 || string(result.Images[0].Data) != "png bytes" {
		t.Errorf("images = %+v, want the decoded bytes", result.Images)
	}

	if result.CostUSD != 0.04 {
		t.Errorf("CostUSD = %v, want the charged 0.04", result.CostUSD)
	}
}

func TestGenerateImagesOmitsUnsetFields(t *testing.T) {
	srv, post := recordingServer(t, `{"data":[{"b64_json":"`+testImageB64+`"}]}`, http.StatusOK)

	if _, err := newTestClient(t, srv.URL).GenerateImages(
		context.Background(), ImageRequest{Prompt: "a lake"},
	); err != nil {
		t.Fatalf("GenerateImages() error: %v", err)
	}

	for _, key := range []string{
		"model", "n", "size", "response_format", "user", "imageDataUrl", "imageDataUrls", "maskDataUrl",
		"strength", "guidance_scale", "num_inference_steps", "seed",
	} {
		post.assertBodyOmits(t, key)
	}
}

func TestGenerateImagesReadsTheBytesHoweverTheyAreWrapped(t *testing.T) {
	raw := []byte("png bytes")

	tests := map[string]string{
		"padded":     base64.StdEncoding.EncodeToString(raw),
		"unpadded":   base64.RawStdEncoding.EncodeToString(raw),
		"data URI":   "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw),
		"whitespace": "\n\t" + base64.StdEncoding.EncodeToString(raw) + "\n",
	}

	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			srv, _ := recordingServer(t, `{"data":[{"b64_json":`+jsonString(encoded)+`}]}`, http.StatusOK)

			result, err := newTestClient(t, srv.URL).GenerateImages(
				context.Background(), ImageRequest{Prompt: "a lake"},
			)
			if err != nil {
				t.Fatalf("GenerateImages() error: %v", err)
			}

			if len(result.Images) != 1 || string(result.Images[0].Data) != string(raw) {
				t.Errorf("images = %+v, want the decoded bytes", result.Images)
			}
		})
	}
}

func TestGenerateImagesKeepsTheURLItFallsBackTo(t *testing.T) {
	srv, _ := recordingServer(t, `{"data":[{"url":"https://cdn.example.com/a.png"}]}`, http.StatusOK)

	result, err := newTestClient(t, srv.URL).GenerateImages(context.Background(), ImageRequest{Prompt: "a lake"})
	if err != nil {
		t.Fatalf("GenerateImages() error: %v", err)
	}

	if len(result.Images) != 1 || result.Images[0].URL != "https://cdn.example.com/a.png" {
		t.Errorf("images = %+v, want the URL the fallback returned", result.Images)
	}

	if result.Images[0].Data != nil {
		t.Errorf("images = %+v, want no bytes alongside a URL", result.Images)
	}
}

func TestGenerateImagesReportsAnUnusableEntry(t *testing.T) {
	tests := map[string]struct {
		response string
		want     string
	}{
		"no images at all": {response: `{"data":[]}`, want: "no images"},
		"neither field":    {response: `{"data":[{"b64_json":"` + testImageB64 + `"},{}]}`, want: "image 2 of 2"},
		"invalid base64":   {response: `{"data":[{"b64_json":"not base64!"}]}`, want: "not base64"},
		"unfinished URI":   {response: `{"data":[{"b64_json":"data:image/png;base64"}]}`, want: "data URI"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv, _ := recordingServer(t, tt.response, http.StatusOK)

			_, err := newTestClient(t, srv.URL).GenerateImages(context.Background(), ImageRequest{Prompt: "a lake"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("GenerateImages() error = %v, want it to carry %q", err, tt.want)
			}
		})
	}
}

func TestGenerateImagesRejectsAnEmptyPrompt(t *testing.T) {
	tests := map[string]ImageRequest{
		"no prompt":    {},
		"blank prompt": {Prompt: "   "},
	}

	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newTestClient(t, freeServer(t)).GenerateImages(context.Background(), req)
			if err == nil || !strings.Contains(err.Error(), "required") {
				t.Errorf("GenerateImages() error = %v, want it to name the missing field", err)
			}
		})
	}
}

func TestGenerateImagesSurfacesAnUpstreamRefusal(t *testing.T) {
	srv, _ := recordingServer(t, `{"error":{"message":"out of credit"}}`, http.StatusPaymentRequired)

	_, err := newTestClient(t, srv.URL).GenerateImages(context.Background(), ImageRequest{Prompt: "a lake"})
	if err == nil || !strings.Contains(err.Error(), "402") || !strings.Contains(err.Error(), "out of credit") {
		t.Errorf("GenerateImages() error = %v, want the status and the message", err)
	}
}

func jsonString(s string) string {
	quoted, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}

	return string(quoted)
}
