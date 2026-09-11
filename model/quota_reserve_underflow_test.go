package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deductions land in the cache first and are persisted afterwards — queued
// entirely under BATCH_UPDATE_ENABLED — so the cached balance is deliberately the
// lower and more current of the two. A user who has been settled past zero must
// therefore stay rejected, and the debt must survive: answering from the database
// row instead would read a balance that has not caught up and hand back spending
// that already happened.
func TestReserveUserQuotaRejectsOverdrawnCacheWithoutConsultingTheLedger(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)

	// The database row still shows the balance the pending deductions have not
	// been applied to yet.
	user := createReserveTestUser(t, 500000)
	require.NoError(t, populateUserCache(user))

	// Settlement overshoots what was reserved and takes the cache past zero.
	result, err := cacheApplyUserQuotaDelta(user.Id, -600000)
	require.NoError(t, err)
	require.Equal(t, cacheQuotaOK, result)

	ok, err := TryReserveUserQuota(user.Id, 1000)
	require.NoError(t, err)
	assert.False(t, ok, "an overdrawn user must not be re-authorized from a lagging database row")

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.Equal(t, -100000, cached.Quota,
		"the debt must survive the rejected reservation, not be refilled from the database")

	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 500000, persisted.Quota, "a rejected reservation must not touch the ledger")

	assert.True(t, server.Exists(getUserCacheKey(user.Id)))
}

// A cached balance that is merely too small is the same decision, answered from
// cache with no database round-trip.
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

// The database stays authoritative when there is no cache entry to be more
// current than it, so a user with a hydrated balance is served normally.
func TestReserveUserQuotaSucceedsFromHydratedBalance(t *testing.T) {
	truncateTables(t)
	useUserCacheMiniRedis(t)

	user := createReserveTestUser(t, 500000)

	ok, err := TryReserveUserQuota(user.Id, 1000)
	require.NoError(t, err)
	assert.True(t, ok)

	var persisted User
	require.NoError(t, DB.First(&persisted, user.Id).Error)
	assert.Equal(t, 499000, persisted.Quota, "the reservation must settle against the ledger exactly once")
}

// The dead hash helpers that guarded writes by reading the TTL first were
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
