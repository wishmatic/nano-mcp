package nanogpt

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const modelsResponseBody = `{
	"object": "list",
	"data": [
		{
			"id": "nano-banana",
			"object": "model",
			"created": 1778544000,
			"owned_by": "google",
			"name": "Nano Banana",
			"description": "Fast image editing.",
			"architecture": {"modality": "text->image", "input_modalities": ["text"], "output_modalities": ["image"]},
			"pricing": {"per_image": {"1024*1024": 0.005}, "currency": "USD"},
			"capabilities": {"image_generation": true, "image_to_image": true, "inpainting": false},
			"supported_parameters": {"resolutions": ["1024x1024", "auto"], "max_images": 4},
			"tags": ["text-to-image", "google"]
		}
	],
	"meta": {"count": 169, "generated_at": "2026-05-14T00:00:00.000Z"}
}`

func TestModelsReadsEachListOverGET(t *testing.T) {
	tests := map[string]struct {
		kind string
		path string
	}{
		"image": {ModelKindImage, "/api/v1/image-models"},
		"video": {ModelKindVideo, "/api/v1/video-models"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			srv, rec := recordingServer(t, modelsResponseBody, http.StatusOK)

			if _, err := newTestClient(t, srv.URL).Models(context.Background(), tt.kind); err != nil {
				t.Fatalf("Models(%s) error: %v", tt.kind, err)
			}

			if rec.method != http.MethodGet {
				t.Errorf("method = %s, want GET", rec.method)
			}

			if rec.path != tt.path {
				t.Errorf("path = %s, want %s", rec.path, tt.path)
			}

			if got := rec.query.Get("detailed"); got != "true" {
				t.Errorf("detailed = %q, want true, which is what carries capabilities and pricing", got)
			}

			rec.assertHeader(t, "Authorization", "Bearer "+testKey)
			rec.assertHeader(t, "x-api-key", "")
		})
	}
}

func TestModelsDecodesTheDetailedFields(t *testing.T) {
	srv, _ := recordingServer(t, modelsResponseBody, http.StatusOK)

	models, err := newTestClient(t, srv.URL).Models(context.Background(), ModelKindImage)
	if err != nil {
		t.Fatalf("Models() error: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("models = %+v, want the one in the response", models)
	}

	model := models[0]
	if model.ID != "nano-banana" || model.Name != "Nano Banana" {
		t.Errorf("model = %+v, want the id and name", model)
	}

	if model.OwnedBy != "google" || model.Description != "Fast image editing." {
		t.Errorf("model = %+v, want the owner and description", model)
	}

	if model.Capabilities["image_to_image"] != true || model.Capabilities["inpainting"] != false {
		t.Errorf("capabilities = %v, want the flags as they arrived", model.Capabilities)
	}

	if got := model.SupportedParameters["max_images"]; got != float64(4) {
		t.Errorf("max_images = %v, want 4", got)
	}

	resolutions, ok := model.SupportedParameters["resolutions"].([]any)
	if !ok || !slices.Equal(resolutions, []any{"1024x1024", "auto"}) {
		t.Errorf("resolutions = %v, want them passed through", model.SupportedParameters["resolutions"])
	}

	perImage, ok := model.Pricing["per_image"].(map[string]any)
	if !ok || perImage["1024*1024"] != 0.005 {
		t.Errorf("pricing = %v, want the per-image prices passed through", model.Pricing)
	}

	if !slices.Equal(model.Tags, []string{"text-to-image", "google"}) {
		t.Errorf("tags = %v, want the upstream tags", model.Tags)
	}
}

func TestModelsRejectsAnUnknownKind(t *testing.T) {
	_, err := newTestClient(t, freeServer(t)).Models(context.Background(), "audio")
	if err == nil || !strings.Contains(err.Error(), "audio") {
		t.Errorf("Models(audio) error = %v, want it to name the kind", err)
	}
}

func TestModelsReportsUpstreamFailures(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv, _ := recordingServer(t, `{"error":"upstream said no"}`, status)

			_, err := newTestClient(t, srv.URL).Models(context.Background(), ModelKindVideo)
			if err == nil || !strings.Contains(err.Error(), strconv.Itoa(status)) {
				t.Errorf("Models() error = %v, want it to name the status", err)
			}

			if err != nil && !strings.Contains(err.Error(), "upstream said no") {
				t.Errorf("error = %q, want the body quoted", err)
			}
		})
	}
}

func TestModelsRejectsAMalformedResponse(t *testing.T) {
	srv, _ := recordingServer(t, `{"data":`, http.StatusOK)

	_, err := newTestClient(t, srv.URL).Models(context.Background(), ModelKindImage)
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("Models() error = %v, want a decode error", err)
	}
}
