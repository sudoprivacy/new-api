package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayErrorAfterKeepalive(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatGemini} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			helper.SetEventStreamHeaders(c)
			require.NoError(t, helper.PingData(c))
			writeRelayError(c, format, types.NewErrorWithStatusCode(errors.New("controlled failure"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest))
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			body := recorder.Body.String()
			require.True(t, strings.HasPrefix(body, ": PING\n\n"), body)
			require.Contains(t, body, "controlled failure")
			require.Contains(t, body, "data:")
			require.True(t, strings.HasSuffix(body, "\n\n"), body)
			if format == types.RelayFormatClaude || format == types.RelayFormatOpenAIResponses {
				require.Contains(t, strings.ReplaceAll(body, ": ", ":"), "event:error\n")
				require.Contains(t, body, `"type":"error"`)
			}
			require.NotContains(t, body, "message_stop")
			require.NotContains(t, body, "[DONE]")
		})
	}
}

func TestRelayErrorBeforeFirstKeepalive(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	helper.SetEventStreamHeaders(c)
	writeRelayError(c, types.RelayFormatClaude, types.NewErrorWithStatusCode(errors.New("bad request"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	require.JSONEq(t, `{"type":"error","error":{"type":"new_api_error","message":"bad request"}}`, recorder.Body.String())
}

func TestRelayErrorAfterClientDisconnect(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	helper.SetEventStreamHeaders(c)
	require.NoError(t, helper.PingData(c))
	before := recorder.Body.String()
	cancel()
	writeRelayError(c, types.RelayFormatClaude, types.NewErrorWithStatusCode(errors.New("failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest))
	require.Equal(t, before, recorder.Body.String())
}
