// sudoapi: Reject Claude streams without an upstream message before billing.

package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeStreamWithoutMessageDoesNotReturnBillableUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, body := range []string{"", ": ping\n\n", "data: {\"type\":\"ping\"}\n\n", "data: {\"type\":\"message_stop\"}\n\n"} {
		t.Run(body, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-5"}}
			info.SetEstimatePromptTokens(2881267)
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
			usage, err := ClaudeStreamHandler(c, resp, info)
			require.NotNil(t, err)
			assert.Nil(t, usage, "no upstream message must not become estimated billable input")
			assert.Equal(t, types.ErrorCodeEmptyResponse, err.GetErrorCode())
		})
	}
}

func TestClaudeInterruptedStreamPreservesReportedUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-5"}}
	body := "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"model\":\"claude-opus-5\",\"usage\":{\"input_tokens\":2,\"output_tokens\":1,\"cache_read_input_tokens\":500}}}\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	usage, err := ClaudeStreamHandler(c, resp, info)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 2, usage.PromptTokens)
	assert.Equal(t, 500, usage.PromptTokensDetails.CachedTokens)
}
