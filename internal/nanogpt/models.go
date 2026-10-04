package nanogpt

import (
	"context"
	"fmt"
	"slices"
)

const (
	// ModelKindImage and ModelKindVideo name the two lists this server reads, and the word each
	// one contributes to nano-gpt's own path.
	ModelKindImage = "image"
	ModelKindVideo = "video"
)

// ModelKinds enumerates the lists, in the order a caller should offer them.
var ModelKinds = []string{ModelKindImage, ModelKindVideo}

// Model is one entry of either list. Capabilities, supported parameters, and pricing stay as
// they arrived, because their shape varies by model and nano-gpt adds fields without notice.
type Model struct {
	ID                  string         `json:"id"`
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	OwnedBy             string         `json:"owned_by"`
	Capabilities        map[string]any `json:"capabilities"`
	SupportedParameters map[string]any `json:"supported_parameters"`
	Pricing             map[string]any `json:"pricing"`
	Tags                []string       `json:"tags"`
}

type modelsResponse struct {
	Data []Model `json:"data"`
}

// Models lists what nano-gpt serves for one kind of media. Which models exist, and what each one
// accepts, changes without notice, which is why it is worth asking rather than hardcoding.
func (c *Client) Models(ctx context.Context, kind string) ([]Model, error) {
	if !slices.Contains(ModelKinds, kind) {
		return nil, fmt.Errorf("model kind %q is not one of %v", kind, ModelKinds)
	}

	var out modelsResponse

	// detailed=true is nano-gpt's own default, stated because it is what carries capabilities,
	// supported parameters, and pricing.
	path := "/api/v1/" + kind + "-models?detailed=true"

	cred := credential{header: "Authorization", value: "Bearer " + c.apiKey}
	if err := c.get(ctx, path, FastTimeout, cred, &out); err != nil {
		return nil, err
	}

	return out.Data, nil
}
