package service

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// One request that leaves this gateway is metered by more than one system, and
// those systems are not redundant copies of each other. Three separate facts are
// being recorded, each with a different owner:
//
//   - What the customer is charged. This gateway owns it, priced from its own
//     ratio tables against the customer's user and token quota. It has to be
//     deterministic and quotable in advance, which means it must not depend on
//     which channel the scheduler happened to pick, or which upstream account
//     that channel happened to use.
//
//   - What the request cost us. The channel owns it. Only the upstream knows its
//     own rates, group multipliers and peak windows, so it bills independently
//     and in its own units — a sub2api channel, for example, settles in USD
//     against its usage log while this gateway settles in quota units.
//
//   - What we have prepaid a channel. The channel owns that too: it is the
//     balance our service account holds there, and it is accounts payable rather
//     than either of the two numbers above.
//
// Keeping all three is correct. What makes them auditable rather than three
// unrelated stories is a shared key, so a row in this gateway's log can be lined
// up with the row the channel wrote for the same request. Without it, per-channel
// margin is unknowable and a pricing or refund change on either side drifts
// silently, because nothing can compare the two ledgers.
//
// This gateway already has that key: Log.RequestId, an indexed column populated
// from the per-request id the request-id middleware assigns. StampCorrelationId
// puts that same value on the upstream request, so the channel records it too
// instead of inventing an id of its own. Nothing new is stored on this side.
const CorrelationIdHeader = "X-Request-ID"

// StampCorrelationId copies this request's id onto the upstream request, under
// the conventional header name, so the channel's own usage log can be joined back
// to Log.RequestId.
//
// It never overwrites a value already on the request: a channel's configured
// header override is an explicit operator decision and outranks this.
func StampCorrelationId(c *gin.Context, req *http.Request) {
	if c == nil || req == nil {
		return
	}
	if req.Header.Get(CorrelationIdHeader) != "" {
		return
	}
	requestId := c.GetString(common.RequestIdKey)
	if requestId == "" {
		return
	}
	req.Header.Set(CorrelationIdHeader, requestId)
}
