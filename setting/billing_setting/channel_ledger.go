// sudoapi: Reconciliation between this gateway's billing and a channel's.

package billing_setting

import (
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
	return ids
}

// UpdateChannelLedgersByJSONString applies the "ChannelLedgers" system option,
// a map of channel id to ledger. Unlike capability data this is operator
// configuration, so the stored value replaces the set outright: removing a
// channel from it must stop reconciling that channel.
func UpdateChannelLedgersByJSONString(jsonStr string) error {
	ledgers := make(map[int]ChannelLedger)
	if jsonStr != "" {
		if err := common.Unmarshal([]byte(jsonStr), &ledgers); err != nil {
			return err
		}
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
