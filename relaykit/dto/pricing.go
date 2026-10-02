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
	ContextWindow        int   `json:"context_window,omitempty"`
	MaxOutputTokens      int   `json:"max_output_tokens,omitempty"`
	VisionSupported      *bool `json:"vision_supported,omitempty"`
	ImageMaxBytes        int   `json:"image_max_bytes,omitempty"`
	ImageMaxDimension    int   `json:"image_max_dimension,omitempty"`
	ToolCallingSupported *bool `json:"tool_calling_supported,omitempty"`
}

type AnthropicModel struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`

	// Anthropic's own /v1/models returns only the four fields above. These are
	// this gateway's extension, carried so an Anthropic-native client learns the
	// same capabilities an OpenAI-shaped one already gets. All omitempty: a
	// consumer that only knows the upstream shape sees a byte-identical response
	// for a model the registry has nothing for, and one that does know these
	// fields can still tell "not known here" from a real value.
	SupportedEndpointTypes []types.EndpointType `json:"supported_endpoint_types,omitempty"`
	ContextWindow          int                  `json:"context_window,omitempty"`
	MaxOutputTokens        int                  `json:"max_output_tokens,omitempty"`
	VisionSupported        *bool                `json:"vision_supported,omitempty"`
	ImageMaxBytes          int                  `json:"image_max_bytes,omitempty"`
	ImageMaxDimension      int                  `json:"image_max_dimension,omitempty"`
	ToolCallingSupported   *bool                `json:"tool_calling_supported,omitempty"`
}

// NewAnthropicModel projects an enriched OpenAIModels onto the Anthropic shape.
//
// The projection lives here, next to both structs, because the bug it replaces
// was a hand-written field-by-field literal at each call site: every capability
// field added to OpenAIModels was silently dropped on this shape, and nothing
// failed. One constructor means a new field is carried or deliberately skipped
// in exactly one place.
func NewAnthropicModel(m OpenAIModels, createdAt string) AnthropicModel {
	return AnthropicModel{
		ID:                     m.Id,
		CreatedAt:              createdAt,
		DisplayName:            m.Id,
		Type:                   "model",
		SupportedEndpointTypes: m.SupportedEndpointTypes,
		ContextWindow:          m.ContextWindow,
		MaxOutputTokens:        m.MaxOutputTokens,
		VisionSupported:        m.VisionSupported,
		ImageMaxBytes:          m.ImageMaxBytes,
		ImageMaxDimension:      m.ImageMaxDimension,
		ToolCallingSupported:   m.ToolCallingSupported,
	}
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
	ToolCallingSupported       *bool         `json:"tool_calling_supported,omitempty"`
}

// NewGeminiModel projects an enriched OpenAIModels onto the Gemini shape.
//
// The two token limits and the gateway's tool capability extension are carried.
// The limit names line up one-to-one with
// what the registry knows, so the mapping is a rename, not an interpretation.
// `supportedGenerationMethods` is deliberately left nil: translating this
// gateway's endpoint types into Gemini method names would be inventing a
// semantic the registry does not record, and a wrong value there tells a client
// a model cannot generate content at all.
//
// A limit the registry has nothing for stays nil, which marshals to `null` —
// unknown, same contract the OpenAI shape expresses by omitting the field. The
// zero check matters: `"inputTokenLimit": 0` would read as a model that accepts
// no input.
func NewGeminiModel(m OpenAIModels) GeminiModel {
	g := GeminiModel{
		Name:                 m.Id,
		DisplayName:          m.Id,
		ToolCallingSupported: m.ToolCallingSupported,
	}
	if m.ContextWindow > 0 {
		g.InputTokenLimit = m.ContextWindow
	}
	if m.MaxOutputTokens > 0 {
		g.OutputTokenLimit = m.MaxOutputTokens
	}
	return g
}
