// sudoapi: Coordinate Claude resets across native channels.

package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// One durable outstanding automatic claim per router. A transport timeout or
// restart leaves the claim fenced until the same peer confirms its outcome.
type ClaudeResetPending struct {
	ID          int `gorm:"primaryKey;autoIncrement:false"`
	Demand      string
	ChannelID   int
	PeerBinding string
	Request     string `gorm:"type:text"`
	PlanBefore  int64
}

func LoadClaudeResetPending(ctx context.Context) (*ClaudeResetPending, error) {
	var pending ClaudeResetPending
	err := DB.WithContext(ctx).First(&pending, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &pending, err
}

func FenceClaudeReset(ctx context.Context, pending ClaudeResetPending) (bool, error) {
	pending.ID = 1
	result := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&pending)
	return result.RowsAffected == 1, result.Error
}

func AuthorizeClaudeResetClaim(ctx context.Context, pending ClaudeResetPending) (bool, error) {
	result := DB.WithContext(ctx).Model(&ClaudeResetPending{}).
		Where("id = ? AND demand = ? AND channel_id = ? AND plan_before > ?", 1, pending.Demand, 0, time.Now().UnixNano()).
		Updates(map[string]any{"channel_id": pending.ChannelID, "peer_binding": pending.PeerBinding, "request": pending.Request})
	return result.RowsAffected == 1, result.Error
}

func ExpireClaudeResetPlan(ctx context.Context, demand string) (bool, error) {
	result := DB.WithContext(ctx).Where("id = ? AND demand = ? AND channel_id = ? AND plan_before <= ?",
		1, demand, 0, time.Now().UnixNano()).Delete(&ClaudeResetPending{})
	return result.RowsAffected == 1, result.Error
}

func SettleClaudeReset(ctx context.Context, demand string) error {
	return DB.WithContext(ctx).Where("id = ? AND demand = ?", 1, demand).Delete(&ClaudeResetPending{}).Error
}
