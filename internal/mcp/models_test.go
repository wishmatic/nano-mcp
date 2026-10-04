package mcp

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

const modelsUpstreamBody = `{
	"object": "list",
	"data": [
		{
			"id": "nano-banana",
			"name": "Nano Banana",
			"owned_by": "google",
			"description": "Fast image editing.",
			"capabilities": {"image_to_image": true, "inpainting": false},
			"supported_parameters": {"resolutions": ["1024x1024"], "max_images": 4},
			"pricing": {"per_image": {"1024*1024": 0.005}, "currency": "USD"},
			"tags": ["editing"]
		},
		{
			"id": "hidream",
			"name": "HiDream",
			"owned_by": "hidream",
			"description": "Illustration and anime.",
			"capabilities": {"image_generation": true},
			"supported_parameters": {"max_images": 1},
			"tags": ["anime"]
		},
		{
			"id": "pruna-ai/p-image/text-to-image",
			"name": "P-Image",
			"description": "Cheap drafts.",
			"tags": ["text-to-image"]
		}
	],
	"meta": {"count": 3}
}`

func TestListModelsSchemaPinsTheKinds(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, "http://127.0.0.1:1")

	properties := toolProperties(t, findTool(t, connectedSession(t, deps), "list_models"))

	if got := properties["kind"].Enum; !slices.Equal(got, nanogpt.ModelKinds) {
		t.Errorf("kind enum = %v, want %v", got, nanogpt.ModelKinds)
	}
}

func TestListModelsReturnsTheModels(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, jsonUpstream(t, "/api/v1/image-models", modelsUpstreamBody))

	result := callTool(t, connectedSession(t, deps), "list_models", map[string]any{"kind": "image"})
	if result.IsError {
		t.Fatalf("list_models failed: %s", errorText(result))
	}

	var out listModelsOutput
	structured(t, result, &out)

	if out.Kind != "image" || out.Total != 3 || out.Matched != 3 || out.Count != 3 {
		t.Errorf("output = %+v, want the whole image list", out)
	}

	if len(out.Models) != 3 {
		t.Fatalf("models = %+v, want one per model nano-gpt listed", out.Models)
	}

	first := out.Models[0]
	if first.ID != "nano-banana" || first.Name != "Nano Banana" || first.OwnedBy != "google" {
		t.Errorf("models[0] = %+v, want the id, name, and owner", first)
	}

	if first.Capabilities["image_to_image"] != true || first.Capabilities["inpainting"] != false {
		t.Errorf("capabilities = %v, want the flags passed through", first.Capabilities)
	}

	if got := first.SupportedParameters["max_images"]; got != float64(4) {
		t.Errorf("max_images = %v, want it passed through", got)
	}

	perImage, ok := first.Pricing["per_image"].(map[string]any)
	if !ok || perImage["1024*1024"] != 0.005 {
		t.Errorf("pricing = %v, want the prices passed through", first.Pricing)
	}

	if !slices.Equal(first.Tags, []string{"editing"}) {
		t.Errorf("tags = %v, want the upstream tags", first.Tags)
	}
}

func TestListModelsReadsTheVideoList(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, jsonUpstream(t, "/api/v1/video-models", modelsUpstreamBody))

	result := callTool(t, connectedSession(t, deps), "list_models", map[string]any{"kind": "video"})
	if result.IsError {
		t.Fatalf("list_models failed: %s", errorText(result))
	}

	var out listModelsOutput
	structured(t, result, &out)

	if out.Kind != "video" || out.Count != 3 {
		t.Errorf("output = %+v, want the video list", out)
	}
}

func TestListModelsFiltersByQuery(t *testing.T) {
	tests := map[string]struct {
		query string
		want  []string
	}{
		"id":          {"nano", []string{"nano-banana"}},
		"tag":         {"ANIME", []string{"hidream"}},
		"description": {"drafts", []string{"pruna-ai/p-image/text-to-image"}},
		"owner":       {"google", []string{"nano-banana"}},
		"no match":    {"gpt-image", nil},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			deps := noopDeps()
			deps.NanoGPT = nanoClient(t, jsonUpstream(t, "/api/v1/image-models", modelsUpstreamBody))

			args := map[string]any{"kind": "image", "query": tt.query}

			result := callTool(t, connectedSession(t, deps), "list_models", args)
			if result.IsError {
				t.Fatalf("list_models failed: %s", errorText(result))
			}

			var out listModelsOutput
			structured(t, result, &out)

			if got := modelIDs(out.Models); !slices.Equal(got, tt.want) {
				t.Errorf("models = %v, want %v for query %q", got, tt.want, tt.query)
			}

			if out.Total != 3 || out.Matched != len(tt.want) || out.Count != len(tt.want) {
				t.Errorf("output = %+v, want the totals counted before and after the query", out)
			}
		})
	}
}

func TestListModelsCapsTheListAtTheLimit(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, jsonUpstream(t, "/api/v1/image-models", modelsUpstreamBody))

	args := map[string]any{"kind": "image", "limit": 2}

	result := callTool(t, connectedSession(t, deps), "list_models", args)
	if result.IsError {
		t.Fatalf("list_models failed: %s", errorText(result))
	}

	var out listModelsOutput
	structured(t, result, &out)

	if out.Count != 2 || len(out.Models) != 2 {
		t.Errorf("output = %+v, want the first two models", out)
	}

	if out.Matched != 3 || out.Total != 3 {
		t.Errorf("output = %+v, want the counts to show the list was cut", out)
	}
}

func TestListModelsRejectsALimitOutsideTheCap(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, "http://127.0.0.1:1")

	for _, limit := range []int{-1, maxModelLimit + 1} {
		args := map[string]any{"kind": "image", "limit": limit}

		result := callTool(t, connectedSession(t, deps), "list_models", args)
		if !result.IsError {
			t.Fatalf("IsError = false, want limit %d rejected", limit)
		}

		if text := errorText(result); !strings.Contains(text, "1 to 100") {
			t.Errorf("error = %q, want the accepted range", text)
		}
	}
}

func TestListModelsReportsUpstreamFailures(t *testing.T) {
	deps := noopDeps()
	deps.NanoGPT = nanoClient(t, upstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))

	result := callTool(t, connectedSession(t, deps), "list_models", map[string]any{"kind": "image"})
	if !result.IsError {
		t.Fatalf("IsError = false, want the upstream 401 to fail the call: %s", errorText(result))
	}

	if text := errorText(result); !strings.Contains(text, "401") || !strings.Contains(text, "bad key") {
		t.Errorf("error = %q, want the status and body", text)
	}
}

func modelIDs(models []modelEntry) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}

	return ids
}
