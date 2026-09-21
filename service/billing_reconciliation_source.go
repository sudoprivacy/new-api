// sudoapi: Reconciliation between this gateway's billing and a channel's.

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

// ChannelChargeSource reads what a channel billed us over a window. Reconcile
// itself takes plain slices; this is what fetches the channel half of them.
type ChannelChargeSource interface {
	ChargesInWindow(ctx context.Context, start, end time.Time) ([]ChannelCharge, error)
}

const (
	// Day granularity is the channel API's, not a choice: it filters by date, so
	// a window that is not whole days would compare a partial day on one side
	// against a full one on the other and report the difference as missing rows.
	reconciliationDay = 24 * time.Hour
	// A window is bounded so one call cannot pull an unbounded history.
	reconciliationMaxDays  = 31
	channelLedgerPageSize  = 100
	channelLedgerMaxPages  = 1000
	channelLedgerTimeout   = 60 * time.Second
	channelLedgerUserAgent = "new-api-reconciliation"
)

// ValidateReconciliationWindow rejects a window the two sides cannot agree on.
func ValidateReconciliationWindow(start, end time.Time) error {
	if !end.After(start) {
		return fmt.Errorf("end must be after start")
	}
	if end.Sub(start)%reconciliationDay != 0 {
		return fmt.Errorf("window must be a whole number of days")
	}
	if end.Sub(start) > reconciliationMaxDays*reconciliationDay {
		return fmt.Errorf("window must not exceed %d days", reconciliationMaxDays)
	}
	return nil
}

// ReconcileWindow compares what this gateway charged over [start, end) with what
// each reconcilable channel charged us for the same requests.
//
// Only channels with a configured ledger take part. A channel we cannot read is
// out of scope rather than an anomaly; including it would report every one of its
// requests as one side having no record.
func ReconcileWindow(ctx context.Context, start, end time.Time) (ReconciliationReport, error) {
	if err := ValidateReconciliationWindow(start, end); err != nil {
		return ReconciliationReport{}, err
	}

	channelIds := billing_setting.GetReconcilableChannelIds()
	if len(channelIds) == 0 {
		return ReconciliationReport{}, nil
	}

	rows, err := model.GetGatewayChargesInWindow(start.Unix(), end.Unix(), channelIds)
	if err != nil {
		return ReconciliationReport{}, fmt.Errorf("read gateway charges: %w", err)
	}
	gatewayCharges := make([]GatewayCharge, 0, len(rows))
	for _, row := range rows {
		gatewayCharges = append(gatewayCharges, GatewayCharge{
			RequestId: row.RequestId,
			ChannelId: row.ChannelId,
			Quota:     row.Quota,
		})
	}

	var channelCharges []ChannelCharge
	for _, channelId := range channelIds {
		ledger, ok := billing_setting.GetChannelLedger(channelId)
		if !ok {
			continue
		}
		charges, err := NewSub2apiChargeSource(ledger).ChargesInWindow(ctx, start, end)
		if err != nil {
			return ReconciliationReport{}, fmt.Errorf("read channel %d ledger: %w", channelId, err)
		}
		channelCharges = append(channelCharges, charges...)
	}

	return Reconcile(gatewayCharges, channelCharges, common.QuotaPerUnit), nil
}

// NewSub2apiChargeSource reads a sub2api channel's usage log over its admin API.
func NewSub2apiChargeSource(ledger billing_setting.ChannelLedger) ChannelChargeSource {
	return &sub2apiChargeSource{
		ledger: ledger,
		client: &http.Client{Timeout: channelLedgerTimeout},
	}
}

type sub2apiChargeSource struct {
	ledger billing_setting.ChannelLedger
	client *http.Client
}

// sub2apiUsagePage mirrors the subset of GET /api/v1/admin/usage this needs.
type sub2apiUsagePage struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Items []struct {
			RequestId  string  `json:"request_id"`
			ActualCost float64 `json:"actual_cost"`
		} `json:"items"`
		Total int64 `json:"total"`
		Pages int   `json:"pages"`
	} `json:"data"`
}

func (s *sub2apiChargeSource) ChargesInWindow(ctx context.Context, start, end time.Time) ([]ChannelCharge, error) {
	// The channel's end_date is inclusive — its handler adds a day to build its
	// own half-open range — so the last included day is the one before our end.
	query := url.Values{}
	query.Set("start_date", start.Format(time.DateOnly))
	query.Set("end_date", end.Add(-reconciliationDay).Format(time.DateOnly))
	query.Set("timezone", start.Location().String())
	query.Set("page_size", strconv.Itoa(channelLedgerPageSize))

	var charges []ChannelCharge
	for page := 1; page <= channelLedgerMaxPages; page++ {
		query.Set("page", strconv.Itoa(page))
		decoded, err := s.fetchPage(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, item := range decoded.Data.Items {
			if item.RequestId == "" {
				continue
			}
			charges = append(charges, ChannelCharge{
				RequestId: item.RequestId,
				CostUSD:   item.ActualCost,
			})
		}
		if len(decoded.Data.Items) < channelLedgerPageSize || page >= decoded.Data.Pages {
			return charges, nil
		}
	}
	return nil, fmt.Errorf("channel ledger did not terminate within %d pages", channelLedgerMaxPages)
}

func (s *sub2apiChargeSource) fetchPage(ctx context.Context, query url.Values) (*sub2apiUsagePage, error) {
	endpoint := s.ledger.BaseURL + "/api/v1/admin/usage?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", s.ledger.AdminKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", channelLedgerUserAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// The status alone, never the body: it is an upstream admin response and
		// can carry account detail that does not belong in our logs.
		return nil, fmt.Errorf("channel ledger returned %s", resp.Status)
	}

	var decoded sub2apiUsagePage
	if err := common.DecodeJson(resp.Body, &decoded); err != nil {
		return nil, fmt.Errorf("decode channel ledger page: %w", err)
	}
	if decoded.Code != 0 {
		return nil, fmt.Errorf("channel ledger reported code %d", decoded.Code)
	}
	return &decoded, nil
}
