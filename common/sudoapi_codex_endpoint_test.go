// sudoapi: GPT-5 Codex endpoint discovery.

package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestGPT5CodexEndpointDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name        string
		channelType int
		model       string
		want        []constant.EndpointType
	}{
		{"azure codex", constant.ChannelTypeAzure, "gpt-5-codex", []constant.EndpointType{constant.EndpointTypeOpenAIResponse}},
		{"openai codex", constant.ChannelTypeOpenAI, "gpt-5-codex", []constant.EndpointType{constant.EndpointTypeOpenAIResponse}},
		{"chat model", constant.ChannelTypeAzure, "gpt-5", []constant.EndpointType{constant.EndpointTypeOpenAI}},
		{"existing responses model", constant.ChannelTypeOpenAI, "o3-pro", []constant.EndpointType{constant.EndpointTypeOpenAIResponse}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, GetEndpointTypesByChannelType(tc.channelType, tc.model))
		})
	}
}
