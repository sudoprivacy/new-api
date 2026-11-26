// sudoapi: Coordinate Claude resets across native channels.

package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCoordinatedClaudeResetFenceSurvivesAmbiguousClaim(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previous := DB
	DB = db
	t.Cleanup(func() {
		DB = previous
		raw, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, raw.Close())
	})
	require.NoError(t, db.AutoMigrate(&ClaudeResetPending{}))
	ctx := t.Context()
	plan := ClaudeResetPending{Demand: "first-demand", PlanBefore: time.Now().Add(time.Minute).UnixNano()}
	acquired, err := FenceClaudeReset(ctx, plan)
	require.NoError(t, err)
	require.True(t, acquired)
	acquired, err = FenceClaudeReset(ctx, ClaudeResetPending{Demand: "concurrent-demand"})
	require.NoError(t, err)
	require.False(t, acquired)
	claim := ClaudeResetPending{Demand: plan.Demand, ChannelID: 115, PeerBinding: "test-binding", Request: "{}"}
	acquired, err = AuthorizeClaudeResetClaim(ctx, claim)
	require.NoError(t, err)
	require.True(t, acquired)
	expired, err := ExpireClaudeResetPlan(ctx, claim.Demand)
	require.NoError(t, err)
	require.False(t, expired)
	require.NoError(t, SettleClaudeReset(ctx, "other-demand"))
	stored, err := LoadClaudeResetPending(ctx)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, claim.Demand, stored.Demand)
	require.Equal(t, claim.ChannelID, stored.ChannelID)
	require.NoError(t, SettleClaudeReset(ctx, claim.Demand))
	stored, err = LoadClaudeResetPending(ctx)
	require.NoError(t, err)
	require.Nil(t, stored)
}
