package dto

import "github.com/QuantumNous/new-api/relaykit/types"

// 这里不好动就不动了，本来想独立出来的（
type OpenAIModels struct {
	Id                     string               `json:"id"`
	Object                 string               `json:"object"`
	Created                int                  `json:"created"`
	OwnedBy                string               `json:"owned_by"`
	SupportedEndpointTypes []types.EndpointType `json:"supported_endpoint_types"`

	// Per-model capacity and image-input capability, filled in from the gateway's
	// metadata registry. Every field is omitted when the registry has nothing for
	// the model, so a consumer can tell "not known here" from a real value and
	// apply its own default. VisionSupported is a pointer for exactly that reason:
	// an absent field means unknown, while an explicit false means the model is
	// documented as text-only, and collapsing the two would present a text-only
	// model as vision-capable.
	ContextWindow     int   `json:"context_window,omitempty"`
	MaxOutputTokens   int   `json:"max_output_tokens,omitempty"`
	VisionSupported   *bool `json:"vision_supported,omitempty"`
	ImageMaxBytes     int   `json:"image_max_bytes,omitempty"`
	ImageMaxDimension int   `json:"image_max_dimension,omitempty"`
}

type AnthropicModel struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

type GeminiModel struct {
	Name                       interface{}   `json:"name"`
	BaseModelId                interface{}   `json:"baseModelId"`
	Version                    interface{}   `json:"version"`
	DisplayName                interface{}   `json:"displayName"`
	Description                interface{}   `json:"description"`
	InputTokenLimit            interface{}   `json:"inputTokenLimit"`
	OutputTokenLimit           interface{}   `json:"outputTokenLimit"`
	SupportedGenerationMethods []interface{} `json:"supportedGenerationMethods"`
	Thinking                   interface{}   `json:"thinking"`
	Temperature                interface{}   `json:"temperature"`
	MaxTemperature             interface{}   `json:"maxTemperature"`
	TopP                       interface{}   `json:"topP"`
	TopK                       interface{}   `json:"topK"`
}
