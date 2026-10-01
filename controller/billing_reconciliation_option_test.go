// sudoapi: Reconciliation between this gateway's billing and a channel's.

package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetOptionsOmitsChannelLedgerCredentials(t *testing.T) {
	previous := common.OptionMap
	common.OptionMap = map[string]string{
		"ChannelLedgers": `{"42":{"base_url":"https://pool.example","admin_key":"private-ledger-key","api_key_id":12}}`,
		"SystemName":     "gateway",
	}
	t.Cleanup(func() { common.OptionMap = previous })
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/option/", nil)
	GetOptions(ctx)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "gateway")
	assert.NotContains(t, response.Body.String(), "ChannelLedgers")
	assert.NotContains(t, response.Body.String(), "private-ledger-key")
}
