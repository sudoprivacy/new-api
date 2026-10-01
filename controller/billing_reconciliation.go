// sudoapi: Reconciliation between this gateway's billing and a channel's.

package controller

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// GetBillingReconciliation reports margin per channel and every request whose two
// billing records do not line up, over a window of whole days.
//
// start_date and end_date are YYYY-MM-DD and both inclusive, matching the channel
// API this reads from so the feature does not carry two date conventions. They
// default to yesterday, the window a nightly run wants. timezone names the
// location the days are cut in and must match on both sides; it defaults to UTC.
func GetBillingReconciliation(c *gin.Context) {
	location := time.UTC
	if name := c.Query("timezone"); name != "" {
		loaded, err := time.LoadLocation(name)
		if err != nil {
			common.ApiErrorMsg(c, "invalid timezone")
			return
		}
		location = loaded
	}

	yesterday := time.Now().In(location).AddDate(0, 0, -1).Format(time.DateOnly)
	start, err := time.ParseInLocation(time.DateOnly, c.DefaultQuery("start_date", yesterday), location)
	if err != nil {
		common.ApiErrorMsg(c, "invalid start_date, use YYYY-MM-DD")
		return
	}
	lastDay, err := time.ParseInLocation(time.DateOnly, c.DefaultQuery("end_date", yesterday), location)
	if err != nil {
		common.ApiErrorMsg(c, "invalid end_date, use YYYY-MM-DD")
		return
	}
	end := lastDay.AddDate(0, 0, 1)

	if err := service.ValidateReconciliationWindow(start, end); err != nil {
		common.ApiError(c, err)
		return
	}

	report, err := service.ReconcileWindow(c.Request.Context(), start, end)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"start_date": start.Format(time.DateOnly),
		"end_date":   lastDay.Format(time.DateOnly),
		"timezone":   location.String(),
		"by_channel": report.ByChannel,
		"coverage":   report.Coverage,
		"totals":     report.Totals,
		"anomalies":  report.Anomalies,
		"anomaly_counts": lo.CountValuesBy(report.Anomalies, func(anomaly service.RequestAnomaly) service.AnomalyKind {
			return anomaly.Kind
		}),
	})
}
