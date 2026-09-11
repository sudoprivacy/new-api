// sudoapi: Per-model capability metadata registry.

package controller

import (
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
