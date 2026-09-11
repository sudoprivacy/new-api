// sudoapi: Reconciliation between this gateway's billing and a channel's.

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testQuotaPerUnit (declared in tiered_settle_test.go) is 500000 quota units to
// the dollar, matching common.QuotaPerUnit, so the numbers below read as dollars.

func anomalyByKind(t *testing.T, report ReconciliationReport, kind AnomalyKind) []RequestAnomaly {
	t.Helper()
	var found []RequestAnomaly
	for _, anomaly := range report.Anomalies {
		if anomaly.Kind == kind {
			found = append(found, anomaly)
		}
	}
	return found
}

func TestReconcileReportsMarginPerChannel(t *testing.T) {
	report := Reconcile(
		[]GatewayCharge{
			{RequestId: "r1", ChannelId: 7, Quota: 500000},  // $1.00 earned
			{RequestId: "r2", ChannelId: 7, Quota: 1000000}, // $2.00 earned
			{RequestId: "r3", ChannelId: 9, Quota: 500000},  // $1.00 earned
		},
		[]ChannelCharge{
			{RequestId: "r1", CostUSD: 0.40},
			{RequestId: "r2", CostUSD: 0.60},
			{RequestId: "r3", CostUSD: 0.90},
		},
		testQuotaPerUnit,
	)

	require.Len(t, report.ByChannel, 2)
	require.Empty(t, report.Anomalies)

	assert.Equal(t, 7, report.ByChannel[0].ChannelId)
	assert.InDelta(t, 3.00, report.ByChannel[0].RevenueUSD, 1e-9)
	assert.InDelta(t, 1.00, report.ByChannel[0].CostUSD, 1e-9)
	assert.InDelta(t, 2.00, report.ByChannel[0].MarginUSD, 1e-9)
	assert.InDelta(t, 66.6666, report.ByChannel[0].MarginPct, 1e-3)
	assert.Equal(t, 2, report.ByChannel[0].MatchedPairs)

	assert.Equal(t, 9, report.ByChannel[1].ChannelId)
	assert.InDelta(t, 0.10, report.ByChannel[1].MarginUSD, 1e-9)

	assert.Equal(t, 3, report.Totals.Requests)
	assert.InDelta(t, 4.00, report.Totals.RevenueUSD, 1e-9)
	assert.InDelta(t, 1.90, report.Totals.CostUSD, 1e-9)
	assert.InDelta(t, 2.10, report.Totals.MarginUSD, 1e-9)
}

// A request the channel billed and this gateway did not is revenue lost outright,
// and it is the anomaly that costs real money, so it must be reported even though
// this side contributes nothing to look at.
func TestReconcileFlagsRequestsTheGatewayNeverBilled(t *testing.T) {
	report := Reconcile(
		[]GatewayCharge{{RequestId: "r1", ChannelId: 7, Quota: 500000}},
		[]ChannelCharge{
			{RequestId: "r1", CostUSD: 0.40},
			{RequestId: "r2", CostUSD: 0.25},
		},
		testQuotaPerUnit,
	)

	unbilled := anomalyByKind(t, report, AnomalyUnbilled)
	require.Len(t, unbilled, 1)
	assert.Equal(t, "r2", unbilled[0].RequestId)
	assert.InDelta(t, 0.25, unbilled[0].CostUSD, 1e-9)
	assert.Zero(t, unbilled[0].RevenueUSD)

	assert.InDelta(t, 0.35, report.Totals.MarginUSD, 1e-9,
		"an unbilled request still drags the margin it cost")
}

func TestReconcileFlagsRequestsTheChannelNeverServed(t *testing.T) {
	report := Reconcile(
		[]GatewayCharge{
			{RequestId: "r1", ChannelId: 7, Quota: 500000},
			{RequestId: "r2", ChannelId: 7, Quota: 250000},
		},
		[]ChannelCharge{{RequestId: "r1", CostUSD: 0.40}},
		testQuotaPerUnit,
	)

	unserved := anomalyByKind(t, report, AnomalyUnserved)
	require.Len(t, unserved, 1)
	assert.Equal(t, "r2", unserved[0].RequestId)
	assert.InDelta(t, 0.50, unserved[0].RevenueUSD, 1e-9)
	assert.Zero(t, unserved[0].CostUSD)
}

func TestReconcileFlagsRequestsThatCostMoreThanTheyEarned(t *testing.T) {
	report := Reconcile(
		[]GatewayCharge{
			{RequestId: "cheap", ChannelId: 7, Quota: 500000},
			{RequestId: "dear", ChannelId: 7, Quota: 50000},
		},
		[]ChannelCharge{
			{RequestId: "cheap", CostUSD: 0.40},
			{RequestId: "dear", CostUSD: 0.30},
		},
		testQuotaPerUnit,
	)

	underpriced := anomalyByKind(t, report, AnomalyUnderpriced)
	require.Len(t, underpriced, 1)
	assert.Equal(t, "dear", underpriced[0].RequestId)
	assert.InDelta(t, 0.10, underpriced[0].RevenueUSD, 1e-9)
	assert.InDelta(t, 0.30, underpriced[0].CostUSD, 1e-9)
}

func TestReconcileFlagsDoubleBilling(t *testing.T) {
	report := Reconcile(
		[]GatewayCharge{{RequestId: "r1", ChannelId: 7, Quota: 500000}},
		[]ChannelCharge{
			{RequestId: "r1", CostUSD: 0.40},
			{RequestId: "r1", CostUSD: 0.40},
		},
		testQuotaPerUnit,
	)

	duplicated := anomalyByKind(t, report, AnomalyDuplicated)
	require.Len(t, duplicated, 1)
	assert.Equal(t, "r1", duplicated[0].RequestId)
	assert.Equal(t, 2, duplicated[0].Occurrences)
	assert.InDelta(t, 0.80, duplicated[0].CostUSD, 1e-9,
		"the doubled cost is counted as recorded, so the margin shows the damage")
	assert.Equal(t, 1, report.Totals.Requests, "a duplicated request is still one request")
}

// A refund is a second row that reverses the first. Netting both sides per request
// before comparing means a refund issued on both sides cancels cleanly, and one
// issued on only one side surfaces as the mismatch it is.
func TestReconcileNetsRefundsPerRequest(t *testing.T) {
	t.Run("refunded on both sides", func(t *testing.T) {
		report := Reconcile(
			[]GatewayCharge{
				{RequestId: "r1", ChannelId: 7, Quota: 500000},
				{RequestId: "r1", ChannelId: 7, Quota: -500000},
			},
			[]ChannelCharge{
				{RequestId: "r1", CostUSD: 0.40},
				{RequestId: "r1", CostUSD: -0.40},
			},
			testQuotaPerUnit,
		)

		assert.Zero(t, report.Totals.RevenueUSD)
		assert.Zero(t, report.Totals.CostUSD)
		assert.Empty(t, anomalyByKind(t, report, AnomalyUnderpriced))
	})

	t.Run("refunded by the gateway only", func(t *testing.T) {
		report := Reconcile(
			[]GatewayCharge{
				{RequestId: "r1", ChannelId: 7, Quota: 500000},
				{RequestId: "r1", ChannelId: 7, Quota: -500000},
			},
			[]ChannelCharge{{RequestId: "r1", CostUSD: 0.40}},
			testQuotaPerUnit,
		)

		underpriced := anomalyByKind(t, report, AnomalyUnderpriced)
		require.Len(t, underpriced, 1,
			"a refund the channel did not match leaves us paying for a request we gave back")
		assert.Zero(t, underpriced[0].RevenueUSD)
		assert.InDelta(t, 0.40, underpriced[0].CostUSD, 1e-9)
	})
}

func TestReconcileHandlesEmptyWindow(t *testing.T) {
	report := Reconcile(nil, nil, testQuotaPerUnit)

	assert.Empty(t, report.ByChannel)
	assert.Empty(t, report.Anomalies)
	assert.Zero(t, report.Totals.Requests)
	assert.Zero(t, report.Totals.MarginPct, "an empty window must not divide by zero revenue")
}
