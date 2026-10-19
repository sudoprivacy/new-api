// sudoapi: Protocol-specific OpenAI usage serialization.

package dto

import (
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesUsageJSONRoundTrip(t *testing.T) {
	wire := `{"id":"resp_1","object":"response","status":"completed","model":"model","output":[{"type":"message","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12,"input_tokens_details":{"cached_tokens":2,"cache_write_tokens":1},"output_tokens_details":{"reasoning_tokens":3}},"metadata":{"key":"value"}}`
	var response OpenAIResponsesResponse
	require.NoError(t, kitutil.Unmarshal([]byte(wire), &response))
	require.NotNil(t, response.Usage)
	assert.Equal(t, 7, response.Usage.InputTokens)
	assert.Equal(t, 5, response.Usage.OutputTokens)
	assert.Equal(t, 12, response.Usage.TotalTokens)
	require.NotNil(t, response.Usage.InputTokensDetails)
	assert.Equal(t, 2, response.Usage.InputTokensDetails.CachedTokens)
	assert.Equal(t, 1, response.Usage.InputTokensDetails.CacheWriteTokens)
	assert.Equal(t, 3, response.Usage.CompletionTokenDetails.ReasoningTokens)
	encoded, err := kitutil.Marshal(response)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, kitutil.Unmarshal(encoded, &got))
	assert.Equal(t, "resp_1", got["id"])
	assert.Equal(t, map[string]any{"key": "value"}, got["metadata"])
	assert.Contains(t, string(encoded), `"text":"hello"`)
	usage, ok := got["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(7), usage["input_tokens"])
	assert.Equal(t, float64(5), usage["output_tokens"])
	assert.Equal(t, float64(12), usage["total_tokens"])
	assert.NotContains(t, usage, "prompt_tokens")
	assert.NotContains(t, usage, "completion_tokens")
	assert.NotContains(t, usage, "billing_usage")
	assert.NotContains(t, usage, "quota")
}

func TestOpenAIResponsesUsageNullAndInvalid(t *testing.T) {
	for _, body := range []string{`{"id":"resp_1"}`, `{"id":"resp_1","usage":null}`} {
		var response OpenAIResponsesResponse
		require.NoError(t, kitutil.Unmarshal([]byte(body), &response))
		assert.Nil(t, response.Usage)
		encoded, err := kitutil.Marshal(response)
		require.NoError(t, err)
		assert.Contains(t, string(encoded), `"usage":null`)
	}
	var response OpenAIResponsesResponse
	assert.Error(t, kitutil.Unmarshal([]byte(`{"usage":{"input_tokens":"invalid"}}`), &response))
}

func TestChatUsageOutputRetainsStrictFieldsAndEnvelope(t *testing.T) {
	response := OpenAITextResponse{
		Id: "chatcmpl_1", Object: "chat.completion",
		Usage: Usage{
			PromptTokens: 7, CompletionTokens: 5, TotalTokens: 12,
			InputTokens: 7, OutputTokens: 5,
			InputTokensDetails:          &InputTokenDetails{CachedTokens: 2},
			ClaudeCacheCreation5mTokens: 1, ClaudeCacheCreation1hTokens: 2,
		},
	}
	encoded, err := kitutil.Marshal(response)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, kitutil.Unmarshal(encoded, &got))
	assert.Equal(t, "chatcmpl_1", got["id"])
	assert.Equal(t, "chat.completion", got["object"])
	usage, ok := got["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(7), usage["prompt_tokens"])
	assert.Equal(t, float64(5), usage["completion_tokens"])
	for _, field := range []string{"input_tokens", "output_tokens", "input_tokens_details", "claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens"} {
		assert.NotContains(t, usage, field)
	}
}
