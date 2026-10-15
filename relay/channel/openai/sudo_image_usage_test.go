// sudoapi: Protocol-specific OpenAI usage serialization.

package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImageJSONUsagePreservesBillingAndBody(t *testing.T) {
	body := `{"data":[{"b64_json":"first"},{"b64_json":"second"}],"usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12,"input_tokens_details":{"image_tokens":4,"text_tokens":3,"cached_tokens":2,"cache_write_tokens":1},"output_tokens_details":{"image_tokens":5}}}`
	c, recorder, response, info := newImageTestContext(t, body, "application/json", false)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 1)
	usage, apiError := OpenaiImageHandler(c, info, response)
	require.Nil(t, apiError)
	require.NotNil(t, usage)
	assert.Equal(t, 7, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	assert.Equal(t, 12, usage.TotalTokens)
	assert.Equal(t, 4, usage.PromptTokensDetails.ImageTokens)
	assert.Equal(t, 3, usage.PromptTokensDetails.TextTokens)
	assert.Equal(t, 2, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 1, usage.PromptTokensDetails.CacheWriteTokens)
	assert.Equal(t, 5, usage.CompletionTokenDetails.ImageTokens)
	assert.Equal(t, 2.0, info.PriceData.OtherRatios()["n"])
	assert.Equal(t, body, recorder.Body.String())
}

func TestOpenAIImageJSONErrorPreservesUpstreamError(t *testing.T) {
	body := `{"error":{"message":"invalid image","type":"invalid_request_error","code":"invalid_image"}}`
	c, _, response, info := newImageTestContext(t, body, "application/json", false)
	response.StatusCode = 400
	usage, apiError := OpenaiImageHandler(c, info, response)
	assert.Nil(t, usage)
	require.NotNil(t, apiError)
	assert.Equal(t, 400, apiError.StatusCode)
	assert.Contains(t, apiError.Error(), "invalid image")
}
