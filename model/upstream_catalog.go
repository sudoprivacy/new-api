// sudoapi: Discover models, capabilities, and reference prices from trusted upstream catalogs.
package model

import (
	"errors"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/upstream_catalog"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var upstreamCatalogWriteMu sync.Mutex

// PersistDiscoveredChannelModels commits routing abilities with the channel
// list. A failed ability update must remain discoverable on the next retry.
func PersistDiscoveredChannelModels(channel *Channel, updateModels bool) error {
	updates := map[string]interface{}{"settings": channel.OtherSettings}
	if !updateModels {
		return DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error
	}
	updates["models"] = channel.Models
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
			return err
		}
		return channel.UpdateAbilities(tx)
	})
}

func MergeUpstreamCatalog(channelID int, incoming map[string]upstream_catalog.Entry) error {
	upstreamCatalogWriteMu.Lock()
	defer upstreamCatalogWriteMu.Unlock()
	var saved string
	err := DB.Transaction(func(tx *gorm.DB) error {
		seed := Option{Key: upstream_catalog.OptionKey, Value: "{}"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var option Option
		if err := lockForUpdate(tx).Where(commonKeyCol+" = ?", upstream_catalog.OptionKey).First(&option).Error; err != nil {
			return err
		}
		entries, err := upstream_catalog.Decode(option.Value)
		if err != nil {
			return err
		}
		for name, entry := range incoming {
			previous, exists := entries[name]
			if exists && previous.ChannelID != channelID {
				continue
			}
			entry.ChannelID = channelID
			// A price registry can lag behind model discovery. Preserve a known
			// price through an incomplete refresh; never replace it with a guess.
			if entry.Pricing == nil {
				entry.Pricing = previous.Pricing
			}
			if entry.ContextWindow == 0 {
				entry.ContextWindow = previous.ContextWindow
			}
			if entry.MaxOutputTokens == 0 {
				entry.MaxOutputTokens = previous.MaxOutputTokens
			}
			if entry.VisionSupported == nil {
				entry.VisionSupported = previous.VisionSupported
			}
			if entry.ToolCallingSupported == nil {
				entry.ToolCallingSupported = previous.ToolCallingSupported
			}
			entries[name] = entry
		}
		body, err := common.Marshal(entries)
		if err != nil {
			return err
		}
		saved = string(body)
		if _, err = upstream_catalog.Decode(saved); err != nil {
			return err
		}
		return tx.Model(&Option{}).Where(commonKeyCol+" = ?", upstream_catalog.OptionKey).Update("value", saved).Error
	})
	if err != nil {
		return err
	}
	if saved == "" {
		return errors.New("upstream catalog was not saved")
	}
	return updateOptionMap(upstream_catalog.OptionKey, saved)
}
