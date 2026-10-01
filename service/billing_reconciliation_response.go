// sudoapi: Reconciliation between this gateway's billing and a channel's.

package service

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/gin-gonic/gin"
)

// CaptureChannelBillingID records sub2api's server-generated billing identity.
// X-Request-ID is only a tracing ID: current sub2api bills client:<UUID> from
// X-Client-Request-ID. Keeping that identity avoids changing its billing dedup.
func CaptureChannelBillingID(c *gin.Context, channelID int, header http.Header) {
	c.Set(common.UpstreamRequestIdKey, header.Get(common.RequestIdKey))
	if _, ok := billing_setting.GetChannelLedger(channelID); !ok {
		return
	}
	id := strings.TrimSpace(header.Get("X-Client-Request-ID"))
	if id == "" || len(id) > 120 {
		return
	}
	c.Set(common.UpstreamRequestIdKey, "client:"+id)
}
