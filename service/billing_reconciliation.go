// sudoapi: Reconciliation between this gateway's billing and a channel's.

package service

import (
	"sort"

	"github.com/QuantumNous/new-api/common"
)

// A request that leaves this gateway is billed twice, by design: here against
// the customer's quota at our rates, and again by the channel at its own rates
// and in its own units (see billing_correlation.go for why those are two facts
// and not one). Both sides key their record by the same correlation id, which is
// what lets the two be compared at all.
//
// Reconciliation is what turns that pairing into an answer. It is deliberately
// not on the request path: it reads both ledgers after the fact, so a channel
// being slow, down, or disagreeing can never delay or fail a user's request.
//
// What it is for is the margin — revenue minus cost, per channel — and the three
// ways the pairing can break, each of which is a real accounting failure rather
// than a metric:
//
//   - a request only one side recorded. Ours only means we charged for something
//     the channel has no record of serving; theirs only means we served without
//     charging, which is revenue lost outright.
//   - a request that cost more than it earned, which is a pricing table that has
//     drifted out of step with the channel's.
//   - a request either side recorded more than once, which is double billing on
//     whichever side counted twice.
//
// Refunds need no special handling: both sides are netted per request before
// comparison, so a refund issued on only one side shows up as the mismatch it is.

// GatewayCharge is one row of this gateway's log: what a request earned, in quota
// units, attributed to the channel that served it.
type GatewayCharge struct {
	RequestId string
	ChannelId int
	Quota     int
}

// ChannelCharge is one row of a channel's own usage log: what the same request
// cost us, in the channel's units (USD).
type ChannelCharge struct {
	ChannelId int
	RequestId string
	CostUSD   float64
}

// ChannelMargin is the reconciled position for one channel over the window.
type ChannelMargin struct {
	ChannelId    int
	Requests     int
	RevenueUSD   float64
	CostUSD      float64
	MarginUSD    float64
	MarginPct    float64
	MatchedPairs int
}

// RequestAnomaly is a single request whose two records do not line up. Cost and
// revenue are in USD so they are directly comparable; a side that has no record
// is reported as zero and named by Kind.
type RequestAnomaly struct {
	RequestId  string
	ChannelId  int
	Kind       AnomalyKind
	RevenueUSD float64
	CostUSD    float64
	// Occurrences is how many rows the duplicating side recorded; it is only
	// meaningful for AnomalyDuplicated.
	Occurrences int
}

type AnomalyKind string

const (
	// AnomalyUnbilled: the channel charged us and this gateway has no record.
	// Revenue lost outright.
	AnomalyUnbilled AnomalyKind = "unbilled"
	// AnomalyUnserved: this gateway charged and the channel has no record of
	// serving it. Either the correlation id did not reach the channel, or the
	// customer was charged for something that never happened.
	AnomalyUnserved AnomalyKind = "unserved"
	// AnomalyUnderpriced: the request cost more than it earned.
	AnomalyUnderpriced AnomalyKind = "underpriced"
	// AnomalyDuplicated: one side recorded the same request more than once.
	AnomalyDuplicated AnomalyKind = "duplicated"
)

// ReconciliationReport is the outcome for one window.
type ReconciliationReport struct {
	Coverage  []ChannelReconciliationCoverage
	ByChannel []ChannelMargin
	Totals    ChannelMargin
	Anomalies []RequestAnomaly
}

// Coverage makes the deployment cutoff explicit instead of implying that older
// rows without an upstream billing ID were successfully reconciled.
type ChannelReconciliationCoverage struct {
	ChannelId int   `json:"channel_id"`
	StartAt   int64 `json:"start_at"`
	EndAt     int64 `json:"end_at"`
}

// Reconcile pairs this gateway's charges with a channel's by correlation id and
// reports the margin and every way the pairing broke.
//
// quotaPerUnit converts our quota units to USD so the two sides are comparable;
// pass common.QuotaPerUnit. Both sides are netted per request first, so refunds
// recorded as separate rows cancel against the charge they reverse.
func Reconcile(gateway []GatewayCharge, channel []ChannelCharge, quotaPerUnit float64) ReconciliationReport {
	if quotaPerUnit <= 0 {
		quotaPerUnit = common.QuotaPerUnit
	}

	revenueUSD := make(map[reconciliationKey]float64, len(gateway))
	gatewayRows := make(map[reconciliationKey]int, len(gateway))
	for _, charge := range gateway {
		key := reconciliationKey{charge.ChannelId, charge.RequestId}
		revenueUSD[key] += float64(charge.Quota) / quotaPerUnit
		if charge.Quota >= 0 {
			gatewayRows[key]++
		}
	}

	costUSD := make(map[reconciliationKey]float64, len(channel))
	channelRows := make(map[reconciliationKey]int, len(channel))
	for _, charge := range channel {
		key := reconciliationKey{charge.ChannelId, charge.RequestId}
		costUSD[key] += charge.CostUSD
		if charge.CostUSD >= 0 {
			channelRows[key]++
		}
	}

	byChannel := make(map[int]*ChannelMargin)
	marginFor := func(channelId int) *ChannelMargin {
		margin, ok := byChannel[channelId]
		if !ok {
			margin = &ChannelMargin{ChannelId: channelId}
			byChannel[channelId] = margin
		}
		return margin
	}

	report := ReconciliationReport{}
	for _, key := range sortedRequestIds(revenueUSD, costUSD) {
		revenue, billed := revenueUSD[key]
		cost, served := costUSD[key]
		channelId, requestId := key.channelId, key.requestId

		margin := marginFor(channelId)
		margin.Requests++
		margin.RevenueUSD += revenue
		margin.CostUSD += cost
		if billed && served {
			margin.MatchedPairs++
		}

		switch {
		case !billed:
			report.Anomalies = append(report.Anomalies, RequestAnomaly{
				RequestId: requestId, ChannelId: channelId,
				Kind: AnomalyUnbilled, CostUSD: cost,
			})
		case !served:
			report.Anomalies = append(report.Anomalies, RequestAnomaly{
				RequestId: requestId, ChannelId: channelId,
				Kind: AnomalyUnserved, RevenueUSD: revenue,
			})
		case cost > revenue:
			report.Anomalies = append(report.Anomalies, RequestAnomaly{
				RequestId: requestId, ChannelId: channelId,
				Kind: AnomalyUnderpriced, RevenueUSD: revenue, CostUSD: cost,
			})
		}

		if occurrences := max(gatewayRows[key], channelRows[key]); occurrences > 1 {
			report.Anomalies = append(report.Anomalies, RequestAnomaly{
				RequestId: requestId, ChannelId: channelId,
				Kind: AnomalyDuplicated, RevenueUSD: revenue, CostUSD: cost,
				Occurrences: occurrences,
			})
		}
	}

	for _, margin := range byChannel {
		finalizeMargin(margin)
		report.ByChannel = append(report.ByChannel, *margin)
		report.Totals.ChannelId = 0
		report.Totals.Requests += margin.Requests
		report.Totals.RevenueUSD += margin.RevenueUSD
		report.Totals.CostUSD += margin.CostUSD
		report.Totals.MatchedPairs += margin.MatchedPairs
	}
	finalizeMargin(&report.Totals)
	sort.Slice(report.ByChannel, func(i, j int) bool {
		return report.ByChannel[i].ChannelId < report.ByChannel[j].ChannelId
	})
	return report
}

func finalizeMargin(margin *ChannelMargin) {
	margin.MarginUSD = margin.RevenueUSD - margin.CostUSD
	if margin.RevenueUSD != 0 {
		margin.MarginPct = margin.MarginUSD / margin.RevenueUSD * 100
	}
}

type reconciliationKey struct {
	channelId int
	requestId string
}

func sortedRequestIds(revenue, cost map[reconciliationKey]float64) []reconciliationKey {
	ids := make([]reconciliationKey, 0, len(revenue)+len(cost))
	for id := range revenue {
		ids = append(ids, id)
	}
	for id := range cost {
		if _, ok := revenue[id]; !ok {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].channelId != ids[j].channelId {
			return ids[i].channelId < ids[j].channelId
		}
		return ids[i].requestId < ids[j].requestId
	})
	return ids
}
