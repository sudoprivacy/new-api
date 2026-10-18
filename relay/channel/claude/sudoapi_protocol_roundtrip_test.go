// sudoapi: Preserve Claude protocol fields and tool inputs.

package claude

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the real native adaptor, HTTP transport and response handler across
// a signed tool turn, an effort change, and a rejected edit to its prefix.
func TestClaudeSignedThinkingProtocolRoundTrip(t *testing.T) {
	service.InitHttpClient()
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		step := requests.Add(1)
		if gjson.GetBytes(data, "thinking.block_binding.prefix_mismatch_behavior").String() != "error" ||
			r.Header.Get("Anthropic-Beta") != "thinking-binding-controls-2026-08-01,mid-conversation-output-config-2026-07-01" {
			http.Error(w, "thinking binding contract was dropped", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if step == 1 {
			_, _ = io.WriteString(w, `{"type":"message","id":"msg_signed","role":"assistant","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"","signature":"signed-empty-fixture"},{"type":"tool_use","id":"sensor_read","name":"sensor","input":{"fraction":0.375,"large":18446744073709551615}}],"stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":40},"input_transformations":[]}`)
			return
		}
		if gjson.GetBytes(data, "messages.1.content.0.signature").String() != "signed-empty-fixture" ||
			gjson.GetBytes(data, "messages.1.content.0.thinking").Raw != `""` ||
			gjson.GetBytes(data, "messages.1.content.1.input.large").Raw != "18446744073709551615" ||
			gjson.GetBytes(data, "messages.1.content.1.input.fraction").Raw != "0.375" ||
			gjson.GetBytes(data, "messages.2.output_config.effort").String() != "low" {
			http.Error(w, "signed history, tool input, or message effort changed", http.StatusBadRequest)
			return
		}
		if gjson.GetBytes(data, "system").String() != "original system" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"Invalid signature in thinking block: bound to a different conversation"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"type":"message","id":"msg_result","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"SENSOR_RESULT:0.375"}],"stop_reason":"end_turn","usage":{"input_tokens":180,"output_tokens":12},"input_transformations":[]}`)
	}))
	defer upstream.Close()

	engine := gin.New()
	engine.POST("/v1/messages", func(c *gin.Context) {
		var input dto.ClaudeRequest
		if err := common.DecodeJson(c.Request.Body, &input); err != nil {
			c.String(http.StatusBadRequest, "%v", err)
			return
		}
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: 14, ChannelBaseUrl: upstream.URL, ApiKey: "local-test-key", UpstreamModelName: input.Model,
		}, RelayFormat: types.RelayFormatClaude}
		adaptor := &Adaptor{}
		converted, err := adaptor.ConvertClaudeRequest(c, info, &input)
		if err != nil {
			c.String(http.StatusBadRequest, "%v", err)
			return
		}
		payload, err := common.Marshal(converted)
		if err != nil {
			c.String(http.StatusInternalServerError, "%v", err)
			return
		}
		response, err := adaptor.DoRequest(c, info, bytes.NewReader(payload))
		if err != nil {
			c.String(http.StatusBadGateway, "%v", err)
			return
		}
		httpResponse := response.(*http.Response)
		if httpResponse.StatusCode != http.StatusOK {
			defer httpResponse.Body.Close()
			c.Status(httpResponse.StatusCode)
			_, _ = io.Copy(c.Writer, httpResponse.Body)
			return
		}
		if _, relayErr := adaptor.DoResponse(c, httpResponse, info); relayErr != nil {
			c.String(http.StatusBadGateway, "%v", relayErr)
		}
	})
	gateway := httptest.NewServer(engine)
	defer gateway.Close()
	post := func(body string) (int, []byte) {
		request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/messages", bytes.NewBufferString(body))
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Anthropic-Beta", "thinking-binding-controls-2026-08-01,mid-conversation-output-config-2026-07-01")
		response, err := gateway.Client().Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		payload, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, payload
	}
	requestPrefix := `"model":"claude-opus-5-5","max_tokens":1024,"thinking":{"type":"adaptive","block_binding":{"prefix_mismatch_behavior":"error"}},"output_config":{"effort":"high"}`
	status, first := post(`{` + requestPrefix + `,"system":"original system","messages":[{"role":"user","content":"Read the sensor."}]}`)
	require.Equal(t, http.StatusOK, status, "%s", first)
	assert.Equal(t, "[]", gjson.GetBytes(first, "input_transformations").Raw)
	content := gjson.GetBytes(first, "content").Raw
	require.NotEmpty(t, content)
	continuation := fmt.Sprintf(`{%s,"system":"%%s","messages":[{"role":"user","content":"Read the sensor."},{"role":"assistant","content":%s},{"role":"system","content":[],"output_config":{"effort":"low"}},{"role":"user","content":[{"type":"tool_result","tool_use_id":"sensor_read","content":"value=0.375"}]}]}`, requestPrefix, content)
	status, second := post(fmt.Sprintf(continuation, "original system"))
	require.Equal(t, http.StatusOK, status, "%s", second)
	assert.Equal(t, "SENSOR_RESULT:0.375", gjson.GetBytes(second, "content.0.text").String())
	assert.Equal(t, "[]", gjson.GetBytes(second, "input_transformations").Raw)
	status, rejected := post(fmt.Sprintf(continuation, "edited system"))
	assert.Equal(t, http.StatusBadRequest, status, "%s", rejected)
	assert.Contains(t, string(rejected), "bound to a different conversation")
	assert.EqualValues(t, 3, requests.Load(), "a signature refusal must not be hidden by a request rewrite or retry")
}
