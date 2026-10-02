// sudoapi: Enforce documented deployment tool capabilities before relay billing.
package helper

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolCapabilityOverHTTP(t *testing.T) {
	original := model_setting.ModelMetadata2JSONString()
	t.Cleanup(func() { require.NoError(t, model_setting.UpdateModelMetadataByJSONString(original)) })
	require.NoError(t, model_setting.UpdateModelMetadataByJSONString(`{"text-route":{"tool_calling_supported":false}}`))
	cases := []struct {
		path   string
		format types.RelayFormat
		body   string
	}{
		{"/v1/chat/completions", types.RelayFormatOpenAI, `{"model":"text-route","messages":[{"role":"user","content":"2+2?"}],"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object"}}}]}`},
		{"/v1/completions", types.RelayFormatOpenAI, `{"model":"text-route","prompt":"2+2?","functions":[{"name":"weather","parameters":{"type":"object"}}]}`},
		{"/v1/messages", types.RelayFormatClaude, `{"model":"text-route","max_tokens":16,"messages":[{"role":"user","content":"2+2?"}],"tools":[{"name":"weather","input_schema":{"type":"object"}}]}`},
		{"/v1/responses", types.RelayFormatOpenAIResponses, `{"model":"text-route","input":"2+2?","tools":[{"type":"function","name":"weather","parameters":{"type":"object"}}]}`},
		{"/v1/responses/compact", types.RelayFormatOpenAIResponsesCompaction, `{"model":"text-route","input":[],"tools":[{"type":"function","name":"weather","parameters":{"type":"object"}}]}`},
		{"/v1beta/models/text-route:generateContent", types.RelayFormatGemini, `{"contents":[{"parts":[{"text":"2+2?"}]}],"tools":[{"functionDeclarations":[{"name":"weather","parameters":{"type":"object"}}]}]}`},
	}
	router := gin.New()
	for _, tc := range cases {
		router.POST(tc.path, func(c *gin.Context) {
			c.Set(string(constant.ContextKeyOriginalModel), "text-route")
			_, err := GetAndValidateRequest(c, tc.format)
			if err != nil {
				c.String(http.StatusBadRequest, "%s", err)
				return
			}
			c.Status(http.StatusNoContent)
		})
	}
	server := httptest.NewServer(router)
	defer server.Close()
	for _, tc := range cases {
		response, err := server.Client().Post(server.URL+tc.path, "application/json", strings.NewReader(tc.body))
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, tc.path)
		assert.Contains(t, string(body), "does not support tool calling", tc.path)
	}
	// Plain chat still works while tools are explicitly unsupported.
	response, err := server.Client().Post(server.URL+cases[0].path, "application/json", strings.NewReader(`{"model":"text-route","messages":[{"role":"user","content":"2+2?"}],"tools":[]}`))
	require.NoError(t, err)
	response.Body.Close()
	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	// An operator's correction reaches the running server. Unknown and true
	// must both retain compatibility with existing tool-capable deployments.
	for _, metadata := range []string{`{}`, `{"text-route":{"tool_calling_supported":true}}`} {
		require.NoError(t, model_setting.UpdateModelMetadataByJSONString(metadata))
		response, err = server.Client().Post(server.URL+cases[0].path, "application/json", strings.NewReader(cases[0].body))
		require.NoError(t, err)
		response.Body.Close()
		assert.Equal(t, http.StatusNoContent, response.StatusCode)
	}
}
