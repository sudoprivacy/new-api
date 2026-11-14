// sudoapi: Enforce explicit token groups for private channels.

package model

import (
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// BeforeCreate also covers batch insertion and server-side copying. The batch
// insertion transaction rolls back all earlier chunks when a channel is invalid.
func (channel *Channel) BeforeCreate(_ *gorm.DB) error {
	return common.ValidateExclusiveChannelGroups(channel.Group)
}
