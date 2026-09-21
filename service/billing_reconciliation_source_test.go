// sudoapi: Reconciliation between this gateway's billing and a channel's.

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation(time.DateOnly, value, time.UTC)
	require.NoError(t, err)
	return parsed
}

// The two sides must cut the window the same way or the difference is reported
// as missing rows. The channel's end_date is inclusive — its handler adds a day
// to build its own half-open range — so ours has to be the day before our end.
func TestSub2apiChargeSourceAsksForTheSameWindow(t *testing.T) {
	var got struct {
		startDate, endDate, timezone, apiKey string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/admin/usage", r.URL.Path)
		got.startDate = r.URL.Query().Get("start_date")
		got.endDate = r.URL.Query().Get("end_date")
		got.timezone = r.URL.Query().Get("timezone")
		got.apiKey = r.Header.Get("x-api-key")
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[],"total":0,"pages":1}}`))
	}))
	defer server.Close()

	source := NewSub2apiChargeSource(billing_setting.ChannelLedger{BaseURL: server.URL, AdminKey: "secret-key"})
	_, err := source.ChargesInWindow(context.Background(), day(t, "2026-09-01"), day(t, "2026-09-04"))
	require.NoError(t, err)

	assert.Equal(t, "2026-09-01", got.startDate)
	assert.Equal(t, "2026-09-03", got.endDate, "the channel's end_date is inclusive")
	assert.Equal(t, "UTC", got.timezone)
	assert.Equal(t, "secret-key", got.apiKey)
}

func TestSub2apiChargeSourceReadsEveryPage(t *testing.T) {
	pages := map[string]string{
		"1": fmt.Sprintf(`{"code":0,"data":{"items":[%s],"total":%d,"pages":2}}`,
			pageItems(1, channelLedgerPageSize), channelLedgerPageSize+1),
		"2": `{"code":0,"data":{"items":[{"request_id":"r-last","actual_cost":0.5}],"total":101,"pages":2}}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Query().Get("page")]
		require.True(t, ok, "unexpected page %q", r.URL.Query().Get("page"))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	source := NewSub2apiChargeSource(billing_setting.ChannelLedger{BaseURL: server.URL})
	charges, err := source.ChargesInWindow(context.Background(), day(t, "2026-09-01"), day(t, "2026-09-02"))
	require.NoError(t, err)

	require.Len(t, charges, channelLedgerPageSize+1)
	assert.Equal(t, "r-last", charges[len(charges)-1].RequestId)
	assert.InDelta(t, 0.5, charges[len(charges)-1].CostUSD, 1e-9)
}

// Rows the channel recorded under an id of its own cannot be joined to anything,
// and counting them would manufacture an anomaly per row.
func TestSub2apiChargeSourceSkipsRowsWithoutARequestId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"items":[
			{"request_id":"r1","actual_cost":0.2},
			{"request_id":"","actual_cost":0.9}
		],"total":2,"pages":1}}`))
	}))
	defer server.Close()

	source := NewSub2apiChargeSource(billing_setting.ChannelLedger{BaseURL: server.URL})
	charges, err := source.ChargesInWindow(context.Background(), day(t, "2026-09-01"), day(t, "2026-09-02"))
	require.NoError(t, err)

	require.Len(t, charges, 1)
	assert.Equal(t, "r1", charges[0].RequestId)
}

// A channel that answers with an error must not be read as "this channel billed
// nothing", which would report every request we billed as unmatched.
func TestSub2apiChargeSourceFailsLoudly(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"http error": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":1001,"message":"INVALID_ADMIN_KEY"}`))
		},
		"application error": func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"code":1001,"message":"INVALID_ADMIN_KEY"}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(handler)
			defer server.Close()

			source := NewSub2apiChargeSource(billing_setting.ChannelLedger{BaseURL: server.URL})
			_, err := source.ChargesInWindow(context.Background(), day(t, "2026-09-01"), day(t, "2026-09-02"))

			require.Error(t, err)
			assert.NotContains(t, err.Error(), "INVALID_ADMIN_KEY",
				"an upstream admin response body must not reach our logs")
		})
	}
}

func TestValidateReconciliationWindow(t *testing.T) {
	for name, tc := range map[string]struct {
		start, end time.Time
		wantErr    string
	}{
		"single day":   {day(t, "2026-09-01"), day(t, "2026-09-02"), ""},
		"empty":        {day(t, "2026-09-01"), day(t, "2026-09-01"), "end must be after start"},
		"reversed":     {day(t, "2026-09-02"), day(t, "2026-09-01"), "end must be after start"},
		"partial day":  {day(t, "2026-09-01"), day(t, "2026-09-02").Add(time.Hour), "whole number of days"},
		"over the cap": {day(t, "2026-09-01"), day(t, "2026-09-01").AddDate(0, 0, reconciliationMaxDays+1), "must not exceed"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateReconciliationWindow(tc.start, tc.end)
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func pageItems(from, count int) string {
	items := ""
	for i := range count {
		if i > 0 {
			items += ","
		}
		items += fmt.Sprintf(`{"request_id":"r%d","actual_cost":0.1}`, from+i)
	}
	return items
}
