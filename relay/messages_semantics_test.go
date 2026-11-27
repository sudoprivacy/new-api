// sudoapi: Require a compatible channel for explicit caller Messages semantics.

package relay

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestMessagesPreferenceRequiresDeclaredRawChannel(t *testing.T) {
	for _, tc := range []struct {
		name, header, body, override string
		raw, allowed                 bool
	}{
		{"legacy", "", "", "", false, true},
		{"header", "exact", "", "{client_header:X-SudoAPI-Messages-Semantics}", true, true},
		{"stock SudoCode", "", "\"sdk-managed\"", "caller", true, true},
		{"public key on wrong channel", "exact", "", "", true, false},
		{"body on wrong channel", "", "\"exact\"", "", true, false},
		{"conversion would drop control", "", "\"sdk-fork\"", "caller", false, false},
		{"mode overridden", "exact", "", "sdk-managed", true, false},
		{"invalid body", "", "null", "caller", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			if tc.header != "" {
				c.Request.Header.Set("X-SudoAPI-Messages-Semantics", tc.header)
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAnthropic, ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: tc.raw}}}
			if tc.override != "" {
				info.HeadersOverride = map[string]any{"X-SudoAPI-Messages-Semantics": tc.override}
			}
			req := &dto.ClaudeRequest{MessagesSemantics: []byte(tc.body)}
			err := checkMessagesSemanticsRoute(c, info, req)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
