// sudoapi: Fail over explicit pre-inference capacity failures within authorized channels.

package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCapacityFailureRequiresUncommittedExplicitAdmissionError(t *testing.T) {
	for _, test := range []struct {
		name, kind, message              string
		status                           int
		committed, pinned, noRetry, want bool
	}{
		{name: "native_capacity", kind: "api_error", message: "No available accounts: no available accounts", status: 503, want: true},
		{name: "native_short_capacity", kind: "api_error", message: "No available accounts", status: 503, want: true},
		{name: "overload_is_uncertain", kind: "overloaded_error", message: "Overloaded", status: 503},
		{name: "transport_is_uncertain", kind: "api_error", message: "Upstream request failed", status: 503},
		{name: "other_status", kind: "api_error", message: "No available accounts", status: 429},
		{name: "started_stream", kind: "api_error", message: "No available accounts", status: 503, committed: true},
		{name: "pinned_channel", kind: "api_error", message: "No available accounts", status: 503, pinned: true},
		{name: "explicit_no_retry", kind: "api_error", message: "No available accounts", status: 503, noRetry: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			if test.committed {
				c.Writer.WriteHeaderNow()
			}
			if test.pinned {
				c.Set("specific_channel_id", 74)
			}
			err := types.WithClaudeError(types.ClaudeError{Type: test.kind, Message: test.message}, test.status)
			if test.noRetry {
				err = types.WithClaudeError(types.ClaudeError{Type: test.kind, Message: test.message}, test.status, types.ErrOptionWithSkipRetry())
			}
			param := &RetryParam{}
			RecordChannelCapacityFailure(c, err, 74, param)
			RecordChannelCapacityFailure(c, err, 74, param)
			if test.want {
				assert.Equal(t, []int{74}, param.ExhaustedChannels)
			} else {
				assert.Empty(t, param.ExhaustedChannels)
			}
		})
	}
}
