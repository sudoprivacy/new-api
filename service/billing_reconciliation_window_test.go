// sudoapi: Reconciliation between this gateway's billing and a channel's.

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReconcileWindowJoinsServerBillingIDAndRespectsCoverage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := model.LOG_DB
	oldLedgers := billing_setting.ChannelLedgers2JSONString()
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = oldDB
		require.NoError(t, billing_setting.UpdateChannelLedgersByJSONString(oldLedgers))
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	start := day(t, "2026-10-01")
	cutoff := start.Unix() + 3600
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "12", r.URL.Query().Get("api_key_id"))
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[
			{"request_id":"client:old","api_key_id":12,"actual_cost":9,"created_at":"2026-10-01T00:30:00Z"},
			{"request_id":"client:served","api_key_id":12,"actual_cost":0.4,"created_at":"2026-10-01T01:30:00Z"}
		],"total":2,"pages":1}}`))
	}))
	defer server.Close()
	require.NoError(t, billing_setting.UpdateChannelLedgersByJSONString(fmt.Sprintf(
		`{"42":{"base_url":%q,"admin_key":"test","api_key_id":12,"start_at":%d}}`, server.URL, cutoff)))
	c := newStampContext(t, "gateway-request")
	CaptureChannelBillingID(c, 42, http.Header{"X-Client-Request-Id": {"served"}})
	assert.Equal(t, "client:served", c.GetString(common.UpstreamRequestIdKey))
	ShouldCopyUpstreamHeader(c, common.RequestIdKey, []string{"tracing-only"})
	assert.Equal(t, "client:served", c.GetString(common.UpstreamRequestIdKey))
	for _, row := range []model.Log{
		{RequestId: "old", ChannelId: 42, CreatedAt: start.Unix() + 1800, Type: model.LogTypeConsume, Quota: 900000},
		{RequestId: "gateway-request", UpstreamRequestId: c.GetString(common.UpstreamRequestIdKey), ChannelId: 42, CreatedAt: cutoff + 1800, Type: model.LogTypeConsume, Quota: 500000},
		{RequestId: "other-channel", ChannelId: 43, CreatedAt: cutoff + 1800, Type: model.LogTypeConsume, Quota: 900000},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	report, err := ReconcileWindow(context.Background(), start, start.AddDate(0, 0, 1))
	require.NoError(t, err)
	assert.Empty(t, report.Anomalies)
	assert.Equal(t, 1, report.Totals.MatchedPairs)
	assert.InDelta(t, 0.6, report.Totals.MarginUSD, 1e-9)
	require.Len(t, report.Coverage, 1)
	assert.Equal(t, cutoff, report.Coverage[0].StartAt)
	CaptureChannelBillingID(c, 43, http.Header{})
	assert.Empty(t, c.GetString(common.UpstreamRequestIdKey), "a retry must clear the prior channel's billing id")
}
