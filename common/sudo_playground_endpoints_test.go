// sudoapi: Chat-compatible Playground model selection.

package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestDecisionEndpointContract(t *testing.T) {
	assert.Equal(t, []constant.EndpointType{"systemone"}, GetEndpointTypesByChannelType(constant.ChannelTypeTypesafe, "MS-Decision-1"))
	assert.NotContains(t, GetEndpointTypesByChannelType(constant.ChannelTypeTypesafe, "jev-latest"), constant.EndpointTypeOpenAI)
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAI}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-4o"))
}
