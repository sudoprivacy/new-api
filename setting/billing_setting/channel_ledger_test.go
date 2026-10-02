// sudoapi: Reconciliation between this gateway's billing and a channel's.

package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelLedgersRequireUnambiguousScope(t *testing.T) {
	old := ChannelLedgers2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateChannelLedgersByJSONString(old)) })
	valid := `{"42":{"base_url":"https://pool.example/","admin_key":"secret","api_key_id":12}}`
	require.NoError(t, UpdateChannelLedgersByJSONString(valid))
	for _, invalid := range []string{
		`{"42":{"base_url":"https://pool.example","admin_key":"secret"}}`,
		`{"42":{"base_url":"https://user:secret@pool.example","admin_key":"secret","api_key_id":12}}`,
		`{"42":{"base_url":"file:///tmp/ledger","admin_key":"secret","api_key_id":12}}`,
		`{"42":{"base_url":"https://pool.example","admin_key":"secret","api_key_id":12},"43":{"base_url":"https://pool.example/","admin_key":"secret","api_key_id":12}}`,
		`{"42":{"base_url":"https://pool.example","admin_key":"secret","api_key_id":12,"previous_api_key_ids":[12]}}`,
		`{"42":{"base_url":"https://pool.example","admin_key":"secret","api_key_id":12,"previous_api_key_ids":[11,11]}}`,
		`{"42":{"base_url":"https://pool.example","admin_key":"secret","api_key_id":12,"previous_api_key_ids":[0]}}`,
		`{"42":{"base_url":"https://pool.example","admin_key":"secret","api_key_id":12,"previous_api_key_ids":[-1]}}`,
		`{"42":{"base_url":"https://pool.example","admin_key":"secret","api_key_id":12,"previous_api_key_ids":[11]},"43":{"base_url":"https://pool.example/","admin_key":"secret","api_key_id":11}}`,
		`{"42":{"base_url":"https://pool.example","admin_key":"secret","api_key_id":12,"previous_api_key_ids":[11]},"43":{"base_url":"https://pool.example/","admin_key":"secret","api_key_id":13,"previous_api_key_ids":[11]}}`,
	} {
		require.Error(t, UpdateChannelLedgersByJSONString(invalid))
		assert.Equal(t, []int{42}, GetReconcilableChannelIds())
	}
	snapshot := GetChannelLedgers()
	delete(snapshot, 42)
	assert.Equal(t, []int{42}, GetReconcilableChannelIds())
	require.NoError(t, UpdateChannelLedgersByJSONString(`{}`))
	assert.Empty(t, GetReconcilableChannelIds())
}

func TestChannelLedgerRotationSnapshotsKeepHistoricalScope(t *testing.T) {
	old := ChannelLedgers2JSONString()
	t.Cleanup(func() { require.NoError(t, UpdateChannelLedgersByJSONString(old)) })
	require.NoError(t, UpdateChannelLedgersByJSONString(`{"42":{"base_url":"https://pool.example","admin_key":"secret","api_key_id":13,"previous_api_key_ids":[12],"start_at":123}}`))
	snapshot := GetChannelLedgers()
	snapshot[42].PreviousAPIKeyIDs[0] = 99
	ledger, ok := GetChannelLedger(42)
	require.True(t, ok)
	assert.Equal(t, []int64{13, 12}, ledger.APIKeyIDs())
	ledger.PreviousAPIKeyIDs[0] = 100
	require.NoError(t, UpdateChannelLedgersByJSONString(ChannelLedgers2JSONString()))
	ledger, _ = GetChannelLedger(42)
	assert.Equal(t, []int64{13, 12}, ledger.APIKeyIDs())
	assert.EqualValues(t, 123, ledger.StartAt)
}
