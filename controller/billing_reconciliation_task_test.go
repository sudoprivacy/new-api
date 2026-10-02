// sudoapi: Reconciliation between this gateway's billing and a channel's.

package controller

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingReconciliationTaskPreservesCountsAndBoundsSamples(t *testing.T) {
	report := service.ReconciliationReport{Anomalies: make([]service.RequestAnomaly, 125)}
	for i := range report.Anomalies {
		report.Anomalies[i] = service.RequestAnomaly{ChannelId: 42, Kind: service.AnomalyUnbilled}
	}
	result, status, err := billingReconciliationTaskResult("2026-09-30", report)
	require.Error(t, err)
	assert.Equal(t, model.SystemTaskStatusFailed, status)
	data := result.(map[string]any)
	assert.Equal(t, 125, data["anomaly_counts"].(map[service.AnomalyKind]int)[service.AnomalyUnbilled])
	assert.Len(t, data["anomaly_samples"], 100)
	_, status, err = billingReconciliationTaskResult("2026-09-30", service.ReconciliationReport{})
	require.NoError(t, err)
	assert.Equal(t, model.SystemTaskStatusSucceeded, status)
}

func TestBillingReconciliationTaskRequiresExplicitEnablement(t *testing.T) {
	t.Setenv("BILLING_RECONCILIATION_TASK_ENABLED", "false")
	assert.False(t, (billingReconciliationHandler{}).Enabled())
}

func TestBillingReconciliationTaskRetainsLargestLossInBoundedSamples(t *testing.T) {
	report := service.ReconciliationReport{}
	for i := range 187 {
		report.Anomalies = append(report.Anomalies, service.RequestAnomaly{
			RequestId: fmt.Sprintf("rounding-%03d", i), ChannelId: 42,
			Kind: service.AnomalyUnderpriced, RevenueUSD: 0.308118, CostUSD: 0.3081185,
		})
	}
	report.Anomalies = append(report.Anomalies, service.RequestAnomaly{
		RequestId: "interrupted-stream", ChannelId: 42,
		Kind: service.AnomalyUnderpriced, RevenueUSD: 0.165048, CostUSD: 0.16947275,
	})
	result, status, err := billingReconciliationTaskResult("2026-10-01", report)
	require.Error(t, err)
	assert.Equal(t, model.SystemTaskStatusFailed, status)
	data := result.(map[string]any)
	samples := data["anomaly_samples"].([]service.RequestAnomaly)
	require.Len(t, samples, 100)
	assert.Equal(t, "interrupted-stream", samples[0].RequestId)
	assert.Equal(t, "rounding-000", samples[1].RequestId)
	assert.Equal(t, 188, data["anomaly_counts"].(map[service.AnomalyKind]int)[service.AnomalyUnderpriced])
	assert.Equal(t, "rounding-000", report.Anomalies[0].RequestId)
	assert.Equal(t, "interrupted-stream", report.Anomalies[187].RequestId)
}
