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
