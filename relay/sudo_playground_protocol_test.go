// sudoapi: Chat-compatible Playground model selection.

package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecisionChatRejectedBeforeUpstream(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeTypesafe)
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "MS-Decision-1")
	var request dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"MS-Decision-1","messages":[{"role":"user","content":"Hello"}]}`), &request))
	apiError := TextHelper(ctx, &relaycommon.RelayInfo{OriginModelName: "MS-Decision-1", Request: &request})
	require.NotNil(t, apiError)
	assert.Equal(t, http.StatusBadRequest, apiError.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiError))
	assert.Contains(t, apiError.Error(), "/v1/systemone")
}
