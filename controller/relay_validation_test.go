// sudoapi: Return client error status for invalid relay requests.

package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayClaudeMessagesRequiredReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/messages",
		strings.NewReader(`{"model":"claude-opus-4-8","messages":[]}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(ctx)

	Relay(ctx, relaytypes.RelayFormatClaude)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Type  string                 `json:"type"`
		Error relaytypes.ClaudeError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "error", response.Type)
	assert.Equal(t, "new_api_error", response.Error.Type)
	assert.Contains(t, response.Error.Message, "field messages is required")
}
