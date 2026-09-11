package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every path that adds quota to the ledger has to move the cached balance with
// it. A reservation that finds the cache short is refused outright — no database
// round-trip, deliberately, so that a stale-high ledger cannot be over-drawn in
// batch mode (see TryReserveUserQuota). The cost of that choice is that a
// stale-*low* cache is believed just as firmly: money that reached the ledger
// without reaching the cache is unspendable until the key expires, and paying in
// again through the same path does not help.
//
// These two tests pin the paths that were missing the sync. They assert the
// symptom a user would report — "I have the money and it still says I don't" —
// rather than the cache write itself, because the write is only interesting for
// what it prevents.

func TestFuiouRechargeMakesCreditSpendableImmediately(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 0)
	require.NoError(t, populateUserCache(user))

	topUp := TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1,
		TradeNo:         "fuiou-" + common.GetRandomString(8),
		PaymentMethod:   PaymentProviderFuiou,
		PaymentProvider: PaymentProviderFuiou,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(&topUp).Error)

	require.NoError(t, RechargeCommon(topUp.TradeNo, "test"))

	credited := common.QuotaFromFloat(topUp.Money * common.QuotaPerUnit)
	require.Greater(t, credited, 0, "the test is meaningless if the top-up credits nothing")

	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	require.Equal(t, credited, persisted.Quota, "the ledger must hold the credit")

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, credited, cached.Quota,
		"the cached balance must follow the ledger, or the credit is unspendable")

	ok, err := TryReserveUserQuota(user.Id, credited)
	require.NoError(t, err)
	assert.True(t, ok, "a user must be able to spend what they just paid in")
}

func TestAffQuotaTransferMakesCreditSpendableImmediately(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	transfer := int(common.QuotaPerUnit)
	user := createReserveTestUser(t, 0)
	user.AffQuota = transfer
	require.NoError(t, DB.Save(&user).Error)
	require.NoError(t, populateUserCache(user))

	require.NoError(t, user.TransferAffQuotaToQuota(transfer))

	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	require.Equal(t, transfer, persisted.Quota, "the ledger must hold the transferred quota")
	require.Equal(t, 0, persisted.AffQuota, "the invite balance must be drawn down")

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, transfer, cached.Quota,
		"the cached balance must follow the ledger, or the transfer is unspendable")

	ok, err := TryReserveUserQuota(user.Id, transfer)
	require.NoError(t, err)
	assert.True(t, ok, "a user must be able to spend quota they just transferred in")
}
