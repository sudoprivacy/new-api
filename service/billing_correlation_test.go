// sudoapi: Correlation id joining this gateway's billing to a channel's.

package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStampContext(t *testing.T, requestId string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if requestId != "" {
		c.Set(common.RequestIdKey, requestId)
	}
	return c
}

// The value stamped upstream has to be the same one Log.RequestId records, or the
// two ledgers still cannot be joined.
func TestStampCorrelationIdMatchesTheLoggedRequestId(t *testing.T) {
	c := newStampContext(t, "req-abc-123")
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	require.NoError(t, err)

	StampCorrelationId(c, req)

	assert.Equal(t, "req-abc-123", req.Header.Get(CorrelationIdHeader))
	assert.Equal(t, c.GetString(common.RequestIdKey), req.Header.Get(CorrelationIdHeader))
}

// A channel's configured header override is an explicit operator decision, and
// header overrides are applied before the request is sent.
func TestStampCorrelationIdKeepsExplicitHeader(t *testing.T) {
	c := newStampContext(t, "req-abc-123")
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	require.NoError(t, err)
	req.Header.Set(CorrelationIdHeader, "operator-supplied")

	StampCorrelationId(c, req)

	assert.Equal(t, "operator-supplied", req.Header.Get(CorrelationIdHeader))
}

func TestStampCorrelationIdSkipsWhenUnset(t *testing.T) {
	c := newStampContext(t, "")
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	require.NoError(t, err)

	StampCorrelationId(c, req)

	assert.Empty(t, req.Header.Values(CorrelationIdHeader),
		"an absent request id must not become an empty header")
}
