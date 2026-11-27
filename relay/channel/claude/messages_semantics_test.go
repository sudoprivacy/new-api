// sudoapi: Preserve caller Messages semantics receipts on native streams.

package claude

import (
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeMessagesSemanticsReceipt(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, mode, digest string
		ping, valid        bool
	}{
		{"headers", "exact", digest, false, true}, {"after keepalive", "sdk-fork", digest, true, true},
		{"unknown mode", "other", digest, false, false}, {"malformed hash", "sdk-managed", "bad", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			if tc.ping {
				_, err := c.Writer.WriteString(": ping\n\n")
				require.NoError(t, err)
				c.Writer.Flush()
			}
			h := make(http.Header)
			h.Set("X-SudoAPI-Messages-Semantics", tc.mode)
			h.Set("X-SudoAPI-Messages-SHA256", tc.digest)
			h.Set("X-Private", "secret")
			forwardMessagesSemanticsReceipt(c, h)
			if tc.valid && !tc.ping {
				require.Equal(t, digest, w.Header().Get("X-SudoAPI-Messages-SHA256"))
				require.Equal(t, tc.mode, w.Header().Get("X-SudoAPI-Messages-Semantics"))
			} else {
				require.Empty(t, w.Header().Get("X-SudoAPI-Messages-SHA256"))
			}
			if tc.valid && tc.ping {
				require.Equal(t, ": ping\n\n: sudoapi-messages-semantics sdk-fork "+digest+"\n\n", w.Body.String())
			} else {
				require.NotContains(t, w.Body.String(), "sudoapi-messages-semantics")
			}
			require.Empty(t, w.Header().Get("X-Private"))
		})
	}
}
