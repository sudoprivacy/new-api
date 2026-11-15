// sudoapi: Complete failed-request refunds before HTTP drain returns.

package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// refundBeforeReturn keeps the existing once-only guard and refund ordering.
// With batch updates disabled, the HTTP handler cannot finish while its refund
// is still waiting in a process-local goroutine. An interrupted database commit
// remains ambiguous; non-idempotent wallet credits must never be blindly retried.
func (s *BillingSession) refundBeforeReturn(c *gin.Context) {
	s.mu.Lock()
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}
	s.refunded = true
	s.mu.Unlock()

	logger.LogInfo(c, fmt.Sprintf("refunding failed request (userId=%d, funding=%s, tokenQuota=%d)",
		s.relayInfo.UserId, s.funding.Source(), s.tokenConsumed))
	if err := s.funding.Refund(); err != nil {
		common.SysLog("error refunding billing source: " + err.Error())
	}
	if s.extraReserved > 0 && s.funding.Source() == BillingSourceSubscription && s.relayInfo.SubscriptionId > 0 {
		if err := model.PostConsumeUserSubscriptionDelta(s.relayInfo.SubscriptionId, -int64(s.extraReserved)); err != nil {
			common.SysLog("error refunding subscription extra reserved quota: " + err.Error())
		}
	}
	if s.tokenConsumed > 0 && !s.relayInfo.IsPlayground {
		if err := model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, s.tokenConsumed); err != nil {
			common.SysLog("error refunding token quota: " + err.Error())
		}
	}
}
