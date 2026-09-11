// sudoapi: Per-model capability metadata registry.

package model_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Clients size a model from what this endpoint reports and fall back to their own
// bundled default when it reports nothing. The current flagship set is exactly the
// part they cannot have seeded locally, so a missing entry there silently caps a
// 1M model at whatever the client guesses.
func TestCurrentFlagshipModelsCarryCapacity(t *testing.T) {
	for _, name := range []string{"claude-opus-5", "claude-sonnet-5", "claude-fable-5-1"} {
		meta := GetModelMetadata(name)
		require.NotNil(t, meta, "%s must be in the registry", name)
		assert.Equal(t, 1000000, meta.ContextWindow, "%s", name)
		assert.Equal(t, 128000, meta.MaxOutputTokens, "%s", name)
		require.NotNil(t, meta.VisionSupported, "%s", name)
		assert.True(t, *meta.VisionSupported, "%s", name)
	}
}

// Capability data describes the models themselves, not operator policy, so an
// option saved before a model existed must not hide it. Treating the stored value
// as the whole table would freeze it at whatever it was saved from and strand
// every model added afterwards — and options are re-applied from the database on
// startup and on every sync tick, so no redeploy would clear it.
func TestOptionOverridesWithoutFreezingTheTable(t *testing.T) {
	original := ModelMetadata2JSONString()
	t.Cleanup(func() { _ = UpdateModelMetadataByJSONString(original) })

	require.NoError(t, UpdateModelMetadataByJSONString(
		`{"claude-opus-4-8":{"context_window":123456,"max_output_tokens":1000}}`))

	overridden := GetModelMetadata("claude-opus-4-8")
	require.NotNil(t, overridden, "the option must win for the model it names")
	assert.Equal(t, 123456, overridden.ContextWindow)

	shipped := GetModelMetadata("claude-opus-5")
	require.NotNil(t, shipped, "a model the stored option predates must survive it")
	assert.Equal(t, 1000000, shipped.ContextWindow)
}

// An override must not be able to rewrite the table this build ships, or the
// next merge would layer on top of already-mutated defaults.
func TestOverridesDoNotMutateShippedDefaults(t *testing.T) {
	original := ModelMetadata2JSONString()
	t.Cleanup(func() { _ = UpdateModelMetadataByJSONString(original) })

	require.NoError(t, UpdateModelMetadataByJSONString(`{"claude-opus-5":{"context_window":1}}`))
	assert.Equal(t, 1000000, defaultModelMetadata["claude-opus-5"].ContextWindow)

	require.NoError(t, UpdateModelMetadataByJSONString(`{}`))
	restored := GetModelMetadata("claude-opus-5")
	require.NotNil(t, restored)
	assert.Equal(t, 1000000, restored.ContextWindow, "clearing the option must restore the shipped value")
}

// Guards the seeded table itself so it cannot drift into a state that reports a
// capability the model does not have.
func TestRegistryInvariants(t *testing.T) {
	all := GetAllModelMetadata()
	require.NotEmpty(t, all)

	for name, meta := range all {
		if meta.ImageMaxBytes != 0 || meta.ImageMaxDimension != 0 {
			require.NotNil(t, meta.VisionSupported, "%s: image caps set but vision_supported unset", name)
			assert.True(t, *meta.VisionSupported, "%s: image caps set but vision_supported is false", name)
		}
		assert.GreaterOrEqual(t, meta.ImageMaxBytes, 0, "%s", name)
		assert.GreaterOrEqual(t, meta.ImageMaxDimension, 0, "%s", name)
		assert.GreaterOrEqual(t, meta.ContextWindow, 0, "%s", name)
	}
}
