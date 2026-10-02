// sudoapi: Reconciliation between this gateway's billing and a channel's.

package billing_setting

import (
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// ChannelLedger is where a channel's own billing can be read from, so its cost
// for a request can be lined up against what we charged for it.
//
// Only channels listed here are reconciled. A channel we cannot read is not an
// anomaly — it is simply out of scope — and including it would report every one
// of its requests as unmatched.
type ChannelLedger struct {
	// BaseURL is the channel gateway's root, e.g. https://sub2api.example.
	BaseURL string `json:"base_url"`
	// AdminKey authenticates against the channel's admin API. It is a secret and
	// is never returned to the client by the reconciliation endpoint.
	AdminKey string `json:"admin_key"`
	// APIKeyID limits the upstream ledger to the key used by this channel.
	APIKeyID int64 `json:"api_key_id"`
	// StartAt excludes history from before billing IDs were captured (Unix seconds).
	StartAt int64 `json:"start_at,omitempty"`
	// PreviousAPIKeyIDs retains the billing history of revoked channel keys.
	// On rotation, move APIKeyID here and set APIKeyID to the replacement key.
	PreviousAPIKeyIDs []int64 `json:"previous_api_key_ids,omitempty"`
}

var (
	channelLedgers      = make(map[int]ChannelLedger)
	channelLedgersMutex sync.RWMutex
)

// GetChannelLedger returns the ledger configured for a channel, if any.
func GetChannelLedger(channelId int) (ChannelLedger, bool) {
	channelLedgersMutex.RLock()
	defer channelLedgersMutex.RUnlock()
	ledger, ok := channelLedgers[channelId]
	ledger.PreviousAPIKeyIDs = slices.Clone(ledger.PreviousAPIKeyIDs)
	return ledger, ok
}

// GetReconcilableChannelIds returns the channels that have a readable ledger.
func GetReconcilableChannelIds() []int {
	channelLedgersMutex.RLock()
	defer channelLedgersMutex.RUnlock()
	ids := make([]int, 0, len(channelLedgers))
	for channelId := range channelLedgers {
		ids = append(ids, channelId)
	}
	sort.Ints(ids)
	return ids
}

// GetChannelLedgers snapshots the configuration for an entire reconciliation.
func GetChannelLedgers() map[int]ChannelLedger {
	channelLedgersMutex.RLock()
	defer channelLedgersMutex.RUnlock()
	result := make(map[int]ChannelLedger, len(channelLedgers))
	for id, ledger := range channelLedgers {
		ledger.PreviousAPIKeyIDs = slices.Clone(ledger.PreviousAPIKeyIDs)
		result[id] = ledger
	}
	return result
}

func (ledger ChannelLedger) Validate() error {
	u, err := url.Parse(ledger.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("ledger base_url must be an HTTP(S) URL without credentials, query or fragment")
	}
	if strings.TrimSpace(ledger.AdminKey) == "" || ledger.APIKeyID <= 0 || ledger.StartAt < 0 {
		return fmt.Errorf("ledger requires admin_key and a positive api_key_id")
	}
	if len(ledger.PreviousAPIKeyIDs) > 32 {
		return fmt.Errorf("ledger supports at most 32 previous API keys")
	}
	seen := map[int64]bool{ledger.APIKeyID: true}
	for _, id := range ledger.PreviousAPIKeyIDs {
		if id <= 0 || seen[id] {
			return fmt.Errorf("ledger API key ids must be positive and distinct")
		}
		seen[id] = true
	}
	return nil
}

// APIKeyIDs includes current and historical credentials without sharing storage
// with the configuration snapshot.
func (ledger ChannelLedger) APIKeyIDs() []int64 {
	return append([]int64{ledger.APIKeyID}, ledger.PreviousAPIKeyIDs...)
}

func parseChannelLedgers(jsonStr string) (map[int]ChannelLedger, error) {
	ledgers := make(map[int]ChannelLedger)
	if jsonStr != "" {
		if err := common.Unmarshal([]byte(jsonStr), &ledgers); err != nil {
			return nil, fmt.Errorf("invalid ChannelLedgers JSON")
		}
	}
	seen := make(map[string]bool)
	for id, ledger := range ledgers {
		if id <= 0 {
			return nil, fmt.Errorf("ledger channel id must be positive")
		}
		if err := ledger.Validate(); err != nil {
			return nil, fmt.Errorf("channel %d: %w", id, err)
		}
		ledger.BaseURL = strings.TrimRight(ledger.BaseURL, "/")
		for _, keyID := range ledger.APIKeyIDs() {
			scope := fmt.Sprintf("%s/%d", ledger.BaseURL, keyID)
			if seen[scope] {
				return nil, fmt.Errorf("each upstream API key must belong to only one reconciled channel")
			}
			seen[scope] = true
		}
		ledgers[id] = ledger
	}
	return ledgers, nil
}

// ValidateChannelLedgersJSONString checks the option before it is persisted.
func ValidateChannelLedgersJSONString(value string) error {
	_, err := parseChannelLedgers(value)
	return err
}

// UpdateChannelLedgersByJSONString applies the "ChannelLedgers" system option,
// a map of channel id to ledger. Unlike capability data this is operator
// configuration, so the stored value replaces the set outright: removing a
// channel from it must stop reconciling that channel.
func UpdateChannelLedgersByJSONString(jsonStr string) error {
	ledgers, err := parseChannelLedgers(jsonStr)
	if err != nil {
		return err
	}
	channelLedgersMutex.Lock()
	defer channelLedgersMutex.Unlock()
	channelLedgers = ledgers
	return nil
}

// ChannelLedgers2JSONString serializes the configured ledgers, secrets included;
// it backs the option value and must not be handed to a client.
func ChannelLedgers2JSONString() string {
	channelLedgersMutex.RLock()
	defer channelLedgersMutex.RUnlock()
	data, _ := common.Marshal(channelLedgers)
	return string(data)
}
