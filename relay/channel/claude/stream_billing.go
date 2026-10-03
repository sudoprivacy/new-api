// sudoapi: Reject Claude streams without an upstream message before billing.

package claude

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/tidwall/gjson"
)

// Pings, comments and a bare message_stop do not establish billable inference.
// Require the upstream message before allowing the partial-output estimator.
func claudeStreamMessageStarted(data string) bool {
	return gjson.Get(data, "type").String() == "message_start" && gjson.Get(data, "message").IsObject()
}

func emptyClaudeStreamError() *types.NewAPIError {
	// The upstream may still be working after the client disconnected. Retrying
	// would start duplicate work; let the existing error path refund any reserve.
	return types.NewErrorWithStatusCode(errors.New("upstream stream ended before a message was received"),
		types.ErrorCodeEmptyResponse, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
}
