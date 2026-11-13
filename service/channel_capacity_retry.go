// sudoapi: Fail over explicit pre-inference capacity failures within authorized channels.

package service

import (
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// A native pool can reject admission before starting inference. Only that
// explicit response releases affinity. Ordinary 503s, timeouts and committed
// streams do not establish that replay is safe. Explicit no-retry and pinned
// channel policies remain authoritative.
func RecordChannelCapacityFailure(c *gin.Context, err *types.NewAPIError, channelID int, param *RetryParam) {
	if c == nil || param == nil || err == nil || channelID <= 0 || c.Writer.Written() ||
		err.StatusCode != http.StatusServiceUnavailable ||
		types.IsSkipRetryError(err) || ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return
	}
	if _, pinned := c.Get("specific_channel_id"); pinned {
		return
	}
	var kind, message string
	switch upstream := err.RelayError.(type) {
	case types.ClaudeError:
		kind, message = upstream.Type, upstream.Message
	case types.OpenAIError: // Non-200 Claude replies use the shared HTTP error parser.
		kind, message = upstream.Type, upstream.Message
	default:
		return
	}
	if kind != "api_error" || (message != "No available accounts" &&
		!strings.HasPrefix(message, "No available accounts: ")) {
		return
	}
	if !slices.Contains(param.ExhaustedChannels, channelID) {
		param.ExhaustedChannels = append(param.ExhaustedChannels, channelID)
	}
	ClearCurrentChannelAffinityCache(c)
}
