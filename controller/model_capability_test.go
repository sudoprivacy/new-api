// sudoapi: Per-model capability metadata registry.

package controller

import (
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// apiModelEntry mirrors the subset of a /v1/models data[] entry that sudocode
// parses. Every field is a pointer on purpose: the contract turns on being able
// to tell an absent field from a zero value, so an absent vision_supported stays
// absent and the consumer applies its own optimistic default instead of reading a
// misleading false.
type apiModelEntry struct {
	ContextWindow     *int  `json:"context_window"`
	MaxOutputTokens   *int  `json:"max_output_tokens"`
	VisionSupported   *bool `json:"vision_supported"`
	ImageMaxBytes     *int  `json:"image_max_bytes"`
	ImageMaxDimension *int  `json:"image_max_dimension"`
}

func enrichAndDecode(t *testing.T, modelName string) apiModelEntry {
	t.Helper()
	m := dto.OpenAIModels{Id: modelName, Object: "model", OwnedBy: "test"}
	enrichModelMetadata(&m)

	encoded, err := common.Marshal(m)
	require.NoError(t, err)
	var entry apiModelEntry
	require.NoError(t, common.Unmarshal(encoded, &entry))
	return entry
}

// End-to-end guard for the capability contract as it reaches the wire: what a
// consumer sees is what survives JSON encoding, not what the struct holds.
func TestModelCapabilityContractOnTheWire(t *testing.T) {
	t.Run("documented vision model reports capacity and image caps", func(t *testing.T) {
		entry := enrichAndDecode(t, "claude-opus-5")

		require.NotNil(t, entry.ContextWindow)
		assert.Equal(t, 1000000, *entry.ContextWindow)
		require.NotNil(t, entry.MaxOutputTokens)
		assert.Equal(t, 128000, *entry.MaxOutputTokens)
		require.NotNil(t, entry.VisionSupported)
		assert.True(t, *entry.VisionSupported)
		require.NotNil(t, entry.ImageMaxBytes)
		assert.Equal(t, 5*1024*1024, *entry.ImageMaxBytes, "Anthropic per-image hard limit is 5 MB")
		require.NotNil(t, entry.ImageMaxDimension)
		assert.Equal(t, 8000, *entry.ImageMaxDimension)
	})

	t.Run("documented text-only model says so explicitly", func(t *testing.T) {
		entry := enrichAndDecode(t, "text-embedding-3-large")

		require.NotNil(t, entry.VisionSupported,
			"a documented text-only model must emit vision_supported:false, not omit it")
		assert.False(t, *entry.VisionSupported)
		assert.Nil(t, entry.ImageMaxBytes, "image caps must be omitted when unknown")
		assert.Nil(t, entry.ImageMaxDimension)
	})

	t.Run("unknown model omits every capability field", func(t *testing.T) {
		entry := enrichAndDecode(t, "some-model-not-in-the-registry")

		assert.Nil(t, entry.ContextWindow)
		assert.Nil(t, entry.MaxOutputTokens)
		assert.Nil(t, entry.VisionSupported,
			"vision_supported must be absent rather than false, so the consumer applies its own default")
		assert.Nil(t, entry.ImageMaxBytes)
		assert.Nil(t, entry.ImageMaxDimension)
	})
}

// enrichedModel is what every shape is projected from: the OpenAI-shaped entry
// after the registry has filled it in. Mirrors what buildOpenAIModel produces.
func enrichedModel(t *testing.T, modelName string) dto.OpenAIModels {
	t.Helper()
	m := dto.OpenAIModels{Id: modelName, Object: "model", Created: 1626777600, OwnedBy: "test"}
	enrichModelMetadata(&m)
	return m
}

func decodeToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	encoded, err := common.Marshal(v)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	return decoded
}

// The OpenAI shape was the only one that carried capability metadata; the
// Anthropic and Gemini projections were hand-written field-by-field literals and
// dropped all of it. These assert on the encoded JSON rather than the structs,
// because a dropped field is invisible at the struct level — the zero value
// encodes as a plausible-looking response.
func TestAlternateModelShapesCarryCapabilities(t *testing.T) {
	t.Run("anthropic shape carries what the registry knows", func(t *testing.T) {
		decoded := decodeToMap(t, dto.NewAnthropicModel(
			enrichedModel(t, "claude-opus-5"), "2021-07-20T00:00:00Z"))

		assert.Equal(t, "claude-opus-5", decoded["id"])
		assert.Equal(t, "model", decoded["type"])
		assert.Equal(t, "2021-07-20T00:00:00Z", decoded["created_at"])
		assert.EqualValues(t, 1000000, decoded["context_window"])
		assert.EqualValues(t, 128000, decoded["max_output_tokens"])
		assert.Equal(t, true, decoded["vision_supported"])
		assert.EqualValues(t, 5*1024*1024, decoded["image_max_bytes"])
		assert.EqualValues(t, 8000, decoded["image_max_dimension"])
	})

	// The extension is only safe if it is invisible when there is nothing to
	// add. A client written against Anthropic's own /v1/models must see the
	// upstream key set exactly, not four fields plus a scatter of zeroes.
	t.Run("anthropic shape for an unknown model is exactly the upstream key set", func(t *testing.T) {
		decoded := decodeToMap(t, dto.NewAnthropicModel(
			enrichedModel(t, "some-model-not-in-the-registry"), "2021-07-20T00:00:00Z"))

		keys := make([]string, 0, len(decoded))
		for k := range decoded {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		assert.Equal(t, []string{"created_at", "display_name", "id", "type"}, keys)
	})

	t.Run("gemini shape carries the token limits under its own names", func(t *testing.T) {
		decoded := decodeToMap(t, dto.NewGeminiModel(enrichedModel(t, "claude-opus-5")))

		assert.Equal(t, "claude-opus-5", decoded["name"])
		assert.EqualValues(t, 1000000, decoded["inputTokenLimit"])
		assert.EqualValues(t, 128000, decoded["outputTokenLimit"])
	})

	// Null means unknown; 0 would claim the model accepts no input and can emit
	// no output, which is worse than saying nothing.
	t.Run("gemini shape leaves unknown limits null rather than zero", func(t *testing.T) {
		decoded := decodeToMap(t, dto.NewGeminiModel(
			enrichedModel(t, "some-model-not-in-the-registry")))

		require.Contains(t, decoded, "inputTokenLimit")
		assert.Nil(t, decoded["inputTokenLimit"])
		assert.Nil(t, decoded["outputTokenLimit"])
	})
}
