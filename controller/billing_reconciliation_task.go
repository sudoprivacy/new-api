// sudoapi: Reconciliation between this gateway's billing and a channel's.

package controller

import (
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

const billingReconciliationTaskType = "billing_reconciliation"

type billingReconciliationHandler struct{}

type billingReconciliationPayload struct {
	Date string `json:"date"`
}

func (billingReconciliationHandler) Type() string { return billingReconciliationTaskType }

func (billingReconciliationHandler) Enabled() bool {
	now := time.Now().UTC()
	if !common.GetEnvOrDefaultBool("BILLING_RECONCILIATION_TASK_ENABLED", false) || now.Hour() < 2 {
		return false
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
	for _, ledger := range billing_setting.GetChannelLedgers() {
		if ledger.StartAt < end {
			return true
		}
	}
	return false
}

func (billingReconciliationHandler) Interval() time.Duration { return 24 * time.Hour }

func (billingReconciliationHandler) NewPayload() any {
	return billingReconciliationPayload{Date: time.Now().UTC().AddDate(0, 0, -1).Format(time.DateOnly)}
}

// The shared system task runner owns the lease and stores the report. Waiting
// until 02:00 UTC gives both ledgers time to settle yesterday's requests.
func (billingReconciliationHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	var payload billingReconciliationPayload
	err := task.DecodePayload(&payload)
	var start time.Time
	if err == nil {
		start, err = time.Parse(time.DateOnly, payload.Date)
	}
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, fmt.Errorf("invalid reconciliation task date"))
		return
	}
	if len(billing_setting.GetReconcilableChannelIds()) == 0 {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, fmt.Errorf("no channel ledgers configured"))
		return
	}
	report, err := service.ReconcileWindow(ctx, start, start.AddDate(0, 0, 1))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	result, status, err := billingReconciliationTaskResult(payload.Date, report)
	if err != nil {
		logger.LogWarn(ctx, err.Error())
	}
	finishSystemTaskHandler(task, runnerID, status, result, err)
}

func billingReconciliationTaskResult(date string, report service.ReconciliationReport) (any, model.SystemTaskStatus, error) {
	counts := make(map[service.AnomalyKind]int)
	for _, anomaly := range report.Anomalies {
		counts[anomaly.Kind]++
	}
	// Keep the largest monetary differences visible in the bounded task report.
	// Sorting by request ID alone can fill all 100 slots with sub-quota rounding
	// differences and omit a real loss. Preserve every anomaly and total in the
	// full report, and leave its deterministic request order unchanged.
	samples := slices.Clone(report.Anomalies)
	slices.SortStableFunc(samples, func(a, b service.RequestAnomaly) int {
		aGap := math.Abs(a.CostUSD - a.RevenueUSD)
		bGap := math.Abs(b.CostUSD - b.RevenueUSD)
		if aGap > bGap {
			return -1
		}
		if aGap < bGap {
			return 1
		}
		return 0
	})
	if len(samples) > 100 {
		samples = samples[:100]
	}
	result := map[string]any{
		"date": date, "timezone": "UTC", "by_channel": report.ByChannel,
		"coverage": report.Coverage,
		"totals":   report.Totals, "anomaly_counts": counts, "anomaly_samples": samples,
	}
	if len(report.Anomalies) > 0 {
		return result, model.SystemTaskStatusFailed, fmt.Errorf("billing reconciliation for %s found %d anomalies; review the task report", date, len(report.Anomalies))
	}
	return result, model.SystemTaskStatusSucceeded, nil
}
