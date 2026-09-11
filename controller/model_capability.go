// sudoapi: Per-model capability metadata registry.

package controller

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

// enrichModelMetadata fills in context window, max output tokens and the
// per-model image-capability fields from the metadata registry. A model the
// registry has never heard of is left untouched so its fields stay off the wire
// and the consumer applies its own default, rather than being told a wrong value.
func enrichModelMetadata(m *dto.OpenAIModels) {
	meta := model_setting.GetModelMetadata(m.Id)
	if meta == nil {
		return
	}
	m.ContextWindow = meta.ContextWindow
	m.MaxOutputTokens = meta.MaxOutputTokens
	m.VisionSupported = meta.VisionSupported
	m.ImageMaxBytes = meta.ImageMaxBytes
	m.ImageMaxDimension = meta.ImageMaxDimension
}
