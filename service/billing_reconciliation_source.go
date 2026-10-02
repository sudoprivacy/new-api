// sudoapi: Reconciliation between this gateway's billing and a channel's.

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
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
	if start.Location().String() != end.Location().String() ||
		start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 || start.Nanosecond() != 0 ||
		end.Hour() != 0 || end.Minute() != 0 || end.Second() != 0 || end.Nanosecond() != 0 {
		return fmt.Errorf("window must be a whole number of days")
	}
	if end.After(start.AddDate(0, 0, reconciliationMaxDays)) {
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := ValidateReconciliationWindow(start, end); err != nil {
		return ReconciliationReport{}, err
	}

	ledgers := billing_setting.GetChannelLedgers()
	channelIds := make([]int, 0, len(ledgers))
	for id := range ledgers {
		channelIds = append(channelIds, id)
	}
	sort.Ints(channelIds)
	if len(channelIds) == 0 {
		return ReconciliationReport{}, nil
	}

	var gatewayCharges []GatewayCharge
	var channelCharges []ChannelCharge
	var coverage []ChannelReconciliationCoverage
	for _, channelId := range channelIds {
		ledger := ledgers[channelId]
		from := min(max(start.Unix(), ledger.StartAt), end.Unix())
		coverage = append(coverage, ChannelReconciliationCoverage{channelId, from, end.Unix()})
		if from == end.Unix() {
			continue
		}
		rows, err := model.GetGatewayChargesInWindow(ctx, from, end.Unix(), []int{channelId})
		if err != nil {
			return ReconciliationReport{}, fmt.Errorf("read gateway charges: %w", err)
		}
		for _, row := range rows {
			requestId := row.RequestId
			if strings.HasPrefix(row.UpstreamRequestId, "client:") {
				requestId = row.UpstreamRequestId
			}
			gatewayCharges = append(gatewayCharges, GatewayCharge{RequestId: requestId, ChannelId: row.ChannelId, Quota: row.Quota})
		}
		charges, err := NewSub2apiChargeSource(ledger).ChargesInWindow(ctx, start, end)
		if err != nil {
			return ReconciliationReport{}, fmt.Errorf("read channel %d ledger: %w", channelId, err)
		}
		for i := range charges {
			charges[i].ChannelId = channelId
		}
		channelCharges = append(channelCharges, charges...)
	}

	report := Reconcile(gatewayCharges, channelCharges, common.QuotaPerUnit)
	report.Coverage = coverage
	return report, nil
}

// NewSub2apiChargeSource reads a sub2api channel's usage log over its admin API.
func NewSub2apiChargeSource(ledger billing_setting.ChannelLedger) ChannelChargeSource {
	return &sub2apiChargeSource{
		ledger: ledger,
		client: &http.Client{Timeout: channelLedgerTimeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}},
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
	Data    *struct {
		Items []struct {
			RequestId  string     `json:"request_id"`
			ActualCost *float64   `json:"actual_cost"`
			APIKeyID   *int64     `json:"api_key_id"`
			CreatedAt  *time.Time `json:"created_at"`
		} `json:"items"`
		Total int64 `json:"total"`
		Pages int   `json:"pages"`
	} `json:"data"`
}

func (s *sub2apiChargeSource) ChargesInWindow(ctx context.Context, start, end time.Time) ([]ChannelCharge, error) {
	if err := ValidateReconciliationWindow(start, end); err != nil {
		return nil, err
	}
	if err := s.ledger.Validate(); err != nil {
		return nil, err
	}
	var charges []ChannelCharge
	for _, keyID := range s.ledger.APIKeyIDs() {
		rows, err := s.chargesForAPIKey(ctx, start, end, keyID)
		if err != nil {
			return nil, err
		}
		charges = append(charges, rows...)
	}
	return charges, nil
}

func (s *sub2apiChargeSource) chargesForAPIKey(ctx context.Context, start, end time.Time, keyID int64) ([]ChannelCharge, error) {
	// The channel's end_date is inclusive — its handler adds a day to build its
	// own half-open range — so the last included day is the one before our end.
	query := url.Values{}
	query.Set("start_date", start.Format(time.DateOnly))
	query.Set("end_date", end.AddDate(0, 0, -1).Format(time.DateOnly))
	query.Set("timezone", start.Location().String())
	query.Set("page_size", strconv.Itoa(channelLedgerPageSize))
	query.Set("api_key_id", strconv.FormatInt(keyID, 10))
	query.Set("exact_total", "true")
	query.Set("sort_by", "id")
	query.Set("sort_order", "asc")

	var charges []ChannelCharge
	for page := 1; page <= channelLedgerMaxPages; page++ {
		query.Set("page", strconv.Itoa(page))
		decoded, err := s.fetchPage(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, item := range decoded.Data.Items {
			if item.APIKeyID == nil || *item.APIKeyID != keyID {
				return nil, fmt.Errorf("channel ledger returned a row outside the configured API key scope")
			}
			if item.ActualCost == nil {
				return nil, fmt.Errorf("channel ledger returned a row without actual_cost")
			}
			if s.ledger.StartAt > 0 {
				if item.CreatedAt == nil {
					return nil, fmt.Errorf("channel ledger returned a row without created_at")
				}
				if item.CreatedAt.Unix() < s.ledger.StartAt {
					continue
				}
			}
			if item.RequestId == "" {
				continue
			}
			charges = append(charges, ChannelCharge{
				RequestId: item.RequestId,
				CostUSD:   *item.ActualCost,
			})
		}
		if decoded.Data.Pages < 0 || (decoded.Data.Pages == 0 && len(decoded.Data.Items) > 0) {
			return nil, fmt.Errorf("channel ledger returned invalid pagination metadata")
		}
		if page >= decoded.Data.Pages {
			return charges, nil
		}
		if len(decoded.Data.Items) != channelLedgerPageSize {
			return nil, fmt.Errorf("channel ledger returned an incomplete page")
		}
	}
	return nil, fmt.Errorf("channel ledger did not terminate within %d pages", channelLedgerMaxPages)
}

func (s *sub2apiChargeSource) fetchPage(ctx context.Context, query url.Values) (*sub2apiUsagePage, error) {
	endpoint := strings.TrimRight(s.ledger.BaseURL, "/") + "/api/v1/admin/usage?" + query.Encode()
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
	if decoded.Data == nil {
		return nil, fmt.Errorf("channel ledger response is missing data")
	}
	return &decoded, nil
}
