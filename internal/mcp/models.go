package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wishmatic/nano-mcp/internal/nanogpt"
)

const (
	defaultModelLimit = 25
	maxModelLimit     = 100
)

type listModelsInput struct {
	Kind  string `json:"kind" jsonschema:"which list to read: the image models are the ids generate_image takes, the video models the ones generate_video takes"`
	Query string `json:"query,omitempty" jsonschema:"keeps only models whose id, name, description, owner, or tags contain this text, ignoring case"`
	Limit int    `json:"limit,omitempty" jsonschema:"how many models to return, 25 by default and 100 at most"`
}

type listModelsOutput struct {
	Kind    string       `json:"kind" jsonschema:"the list that was read"`
	Total   int          `json:"total" jsonschema:"models nano-gpt lists for this kind"`
	Matched int          `json:"matched" jsonschema:"models that match query, before limit"`
	Count   int          `json:"count" jsonschema:"models returned"`
	Models  []modelEntry `json:"models" jsonschema:"the models, in the order nano-gpt lists them"`
}

type modelEntry struct {
	ID                  string         `json:"id" jsonschema:"model id to pass to the generate tools"`
	Name                string         `json:"name,omitempty" jsonschema:"human-readable name"`
	Description         string         `json:"description,omitempty" jsonschema:"what the model is for"`
	OwnedBy             string         `json:"ownedBy,omitempty" jsonschema:"provider that serves it"`
	Capabilities        map[string]any `json:"capabilities,omitempty" jsonschema:"feature flags, such as image_to_image or text_to_video"`
	SupportedParameters map[string]any `json:"supportedParameters,omitempty" jsonschema:"model-specific settings, such as resolutions, max_images, or durations"`
	Pricing             map[string]any `json:"pricing,omitempty" jsonschema:"USD pricing, whose shape varies by model"`
	Tags                []string       `json:"tags,omitempty" jsonschema:"nano-gpt's own labels for the model"`
}

func registerListModels(srv *mcp.Server, h *handlers) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "list_models",
		Description: "List the image or video models nano-gpt currently serves, with their capabilities, " +
			"supported settings, and pricing. Free to call, and the way to find an id for generate_image " +
			"or generate_video rather than guessing one, since the names and the set of models change.",
		InputSchema: mustEnumInputSchema[listModelsInput](map[string][]string{"kind": nanogpt.ModelKinds}),
	}, h.listModels)
}

func (h *handlers) listModels(ctx context.Context, _ *mcp.CallToolRequest, in listModelsInput) (
	*mcp.CallToolResult, listModelsOutput, error,
) {
	limit, err := modelLimit(in.Limit)
	if err != nil {
		return nil, listModelsOutput{}, h.fail("list_models", err)
	}

	models, err := h.nanogpt.Models(ctx, in.Kind)
	if err != nil {
		return nil, listModelsOutput{}, h.fail("list_models", err)
	}

	matched := filterModels(models, in.Query)

	return nil, newListModelsOutput(in.Kind, len(models), matched, limit), nil
}

func modelLimit(limit int) (int, error) {
	switch {
	case limit == 0:
		return defaultModelLimit, nil
	case limit < 0 || limit > maxModelLimit:
		return 0, fmt.Errorf("limit must be 1 to %d, got %d", maxModelLimit, limit)
	default:
		return limit, nil
	}
}

func filterModels(models []nanogpt.Model, query string) []nanogpt.Model {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return models
	}

	matched := make([]nanogpt.Model, 0, len(models))

	for _, model := range models {
		if modelMatches(model, needle) {
			matched = append(matched, model)
		}
	}

	return matched
}

func modelMatches(model nanogpt.Model, needle string) bool {
	fields := append([]string{model.ID, model.Name, model.Description, model.OwnedBy}, model.Tags...)

	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}

	return false
}

func newListModelsOutput(
	kind string, total int, matched []nanogpt.Model, limit int,
) listModelsOutput {
	shown := matched
	if len(shown) > limit {
		shown = shown[:limit]
	}

	models := make([]modelEntry, 0, len(shown))
	for _, model := range shown {
		models = append(models, newModelEntry(model))
	}

	return listModelsOutput{
		Kind:    kind,
		Total:   total,
		Matched: len(matched),
		Count:   len(models),
		Models:  models,
	}
}

func newModelEntry(model nanogpt.Model) modelEntry {
	return modelEntry{
		ID:                  model.ID,
		Name:                model.Name,
		Description:         model.Description,
		OwnedBy:             model.OwnedBy,
		Capabilities:        model.Capabilities,
		SupportedParameters: model.SupportedParameters,
		Pricing:             model.Pricing,
		Tags:                model.Tags,
	}
}
