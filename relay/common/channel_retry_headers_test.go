// sudoapi: Scope outbound header overrides to each relay attempt.
package common

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryUsesSelectedChannelHeaders(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	info := &RelayInfo{}
	ctx.Set(string(constant.ContextKeyChannelId), 1)
	ctx.Set(string(constant.ContextKeyChannelHeaderOverride), map[string]interface{}{"x-first": "one"})
	ctx.Set(string(constant.ContextKeyChannelParamOverride), map[string]interface{}{"temperature": 1})
	info.InitChannelMeta(ctx)
	_, err := ApplyParamOverrideWithRelayInfo([]byte(`{}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)

	// Exercise both the direct path and the affinity template that recompiles overrides.
	ctx.Set(string(constant.ContextKeyChannelId), 2)
	nextHeaders := map[string]interface{}{"x-next": "two"}
	ctx.Set(string(constant.ContextKeyChannelHeaderOverride), nextHeaders)
	info.InitChannelMeta(ctx)
	assert.Equal(t, nextHeaders, GetEffectiveHeaderOverride(info))
	_, err = ApplyParamOverrideWithRelayInfo([]byte(`{}`), info)
	require.NoError(t, err)
	assert.Equal(t, nextHeaders, GetEffectiveHeaderOverride(info))

	ctx.Set(string(constant.ContextKeyChannelHeaderOverride), map[string]interface{}{})
	info.InitChannelMeta(ctx)
	assert.Empty(t, GetEffectiveHeaderOverride(info))
}
