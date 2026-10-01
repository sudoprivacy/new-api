// sudoapi: Reconciliation between this gateway's billing and a channel's.

package controller

import (
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
