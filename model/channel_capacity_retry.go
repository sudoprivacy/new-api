// sudoapi: Fail over explicit pre-inference capacity failures within authorized channels.

package model

import (
	"slices"

	"gorm.io/gorm"
)

// Keep the cached candidate slice immutable; it is shared by concurrent requests.
func excludeExhaustedChannels(channels, exhausted []int) []int {
	if len(exhausted) == 0 {
		return channels
	}
	remaining := make([]int, 0, len(channels))
	for _, id := range channels {
		if !slices.Contains(exhausted, id) {
			remaining = append(remaining, id)
		}
	}
	return remaining
}

func capacityRetryChannelQuery(group, model string, exhausted []int) *gorm.DB {
	maxPriority := DB.Model(&Ability{}).Select("MAX(priority)").
		Where(commonGroupCol+" = ? AND model = ? AND enabled = ?", group, model, true).
		Where("channel_id NOT IN ?", exhausted)
	return DB.Where(commonGroupCol+" = ? AND model = ? AND enabled = ?", group, model, true).
		Where("channel_id NOT IN ?", exhausted).Where("priority = (?)", maxPriority)
}
