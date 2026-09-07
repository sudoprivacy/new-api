package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Settlement and subscription charges apply their delta to the cached balance
// without a floor, so an overshoot can leave the hash negative while the database
// still holds the money. A negative cached balance must not be read as "out of
// quota" — that would reject a funded user until the key expired.
func TestReserveUserQuotaRecoversFromCacheUnderflow(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 500000)
	require.NoError(t, populateUserCache(user))

	// Drive the cached counter below zero the way an overshooting settlement does.
	result, err := cacheApplyUserQuotaDelta(user.Id, -600000)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)

	cached, err := cacheGetUserBase(user.Id)
	require.Error(t, err, "an underflowed balance must not be served")
	require.Nil(t, cached)

	ok, err := TryReserveUserQuota(user.Id, 1000)
	require.NoError(t, err)
	assert.True(t, ok, "a funded user must not be rejected because the cache underflowed")

	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 499000, persisted.Quota, "the reservation must settle against the ledger exactly once")
}

// A cached balance that is merely too small is a real insufficiency and must stay
// one: it is reported without a database round-trip and without rehydrating.
func TestReserveUserQuotaStillRejectsGenuineShortfall(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 100)
	require.NoError(t, populateUserCache(user))

	ok, err := TryReserveUserQuota(user.Id, 1000)
	require.NoError(t, err)
	assert.False(t, ok)

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 100, cached.Quota, "a rejected reservation must not move the balance")
}

// The dead hash helpers that used to guard writes by reading the TTL first were
// removed; every quota mutation now goes through the guarded Lua scripts. This
// pins the property they existed to provide: charging a user whose hash is absent
// must not conjure one holding nothing but a negative quota, because a partial
// hash decodes with every other field at its zero value.
func TestQuotaDeltaLeavesMissingCacheAbsent(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 500000)

	result, err := cacheApplyUserQuotaDelta(user.Id, -1000)
	require.NoError(t, err)
	assert.Equal(t, cacheQuotaMiss, result)
	assert.False(t, server.Exists(getUserCacheKey(user.Id)),
		"a charge must never bring a cache entry into existence")

	quota, err := GetUserQuota(user.Id, false)
	require.NoError(t, err)
	assert.Equal(t, 500000, quota, "the balance still comes from the ledger")
}
