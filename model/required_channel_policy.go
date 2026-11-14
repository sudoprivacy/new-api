// sudoapi: Protect declared gateway channel routes.

package model

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

var ErrRequiredChannelRoute = errors.New("this declared route must retain its channel, groups and models; update the operator routing policy before retiring it")
var ErrRequiredChannelPolicy = errors.New("required channel routing policy is unavailable or invalid; configuration changes are blocked")

type requiredChannelRoute struct {
	groups, models map[string]bool
	private        bool
}

type requiredChannelPolicy struct {
	Gateway       string `json:"gateway"`
	PrivateGroups []struct {
		Name               string   `json:"name"`
		AllowedUserIDs     []int    `json:"allowed_user_ids"`
		RequiredChannelIDs []int    `json:"required_channel_ids"`
		RequiredModels     []string `json:"required_models"`
	} `json:"private_groups"`
	SharedChannels []struct {
		ChannelID      int      `json:"channel_id"`
		RequiredGroups []string `json:"required_groups"`
		RequiredModels []string `json:"required_models"`
	} `json:"shared_channels"`
	routes       map[int]requiredChannelRoute
	privateNames map[string]bool
}

func requiredRouteNames(values []string) (map[string]bool, error) {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.Contains(value, ",") || result[value] {
			return nil, ErrRequiredChannelPolicy
		}
		result[value] = true
	}
	if len(result) == 0 {
		return nil, ErrRequiredChannelPolicy
	}
	return result, nil
}

// The reviewed Ladder declaration is mounted read-only, not copied into Go
// constants. Only configuration writes read it; inference and accounting do not.
func loadRequiredChannelPolicy() (*requiredChannelPolicy, error) {
	path := os.Getenv("REQUIRED_CHANNEL_POLICY_FILE")
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, ErrRequiredChannelPolicy
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 1<<20 {
		return nil, ErrRequiredChannelPolicy
	}
	var p requiredChannelPolicy
	if common.Unmarshal(data, &p) != nil || !strings.HasPrefix(p.Gateway, "https://") {
		return nil, ErrRequiredChannelPolicy
	}
	p.routes, p.privateNames = map[int]requiredChannelRoute{}, map[string]bool{}
	for _, scope := range p.PrivateGroups {
		groups, err := requiredRouteNames([]string{scope.Name})
		if err != nil || p.privateNames[scope.Name] || len(scope.AllowedUserIDs) == 0 || len(scope.RequiredChannelIDs) == 0 {
			return nil, ErrRequiredChannelPolicy
		}
		if _, ok := common.ExplicitTokenGroups()[scope.Name]; !ok {
			return nil, ErrRequiredChannelPolicy
		}
		p.privateNames[scope.Name] = true
		models, err := requiredRouteNames(scope.RequiredModels)
		if err != nil {
			return nil, err
		}
		for _, id := range scope.RequiredChannelIDs {
			if _, exists := p.routes[id]; id <= 0 || exists {
				return nil, ErrRequiredChannelPolicy
			}
			p.routes[id] = requiredChannelRoute{groups: groups, models: models, private: true}
		}
	}
	for _, scope := range p.SharedChannels {
		if _, exists := p.routes[scope.ChannelID]; scope.ChannelID <= 0 || exists {
			return nil, ErrRequiredChannelPolicy
		}
		groups, err := requiredRouteNames(scope.RequiredGroups)
		if err != nil {
			return nil, err
		}
		models, err := requiredRouteNames(scope.RequiredModels)
		if err != nil {
			return nil, err
		}
		for name := range groups {
			if p.privateNames[name] {
				return nil, ErrRequiredChannelPolicy
			}
		}
		p.routes[scope.ChannelID] = requiredChannelRoute{groups: groups, models: models}
	}
	if len(p.routes) == 0 {
		return nil, ErrRequiredChannelPolicy
	}
	return &p, nil
}

func (p *requiredChannelPolicy) validate(current, candidate *Channel) error {
	route, required := p.routes[current.Id]
	if !required {
		return nil
	}
	// Genuine provider health transitions remain possible. A configuration write
	// may preserve an existing auto-disable, but cannot manufacture a new one.
	statusOK := candidate.Status == common.ChannelStatusEnabled ||
		current.Status == common.ChannelStatusAutoDisabled && candidate.Status == current.Status
	if candidate.Type != constant.ChannelTypeAnthropic || !statusOK {
		return fmt.Errorf("%w (channel %d)", ErrRequiredChannelRoute, current.Id)
	}
	groups := make(map[string]bool)
	for _, name := range strings.Split(candidate.Group, ",") {
		if name = strings.TrimSpace(name); name != "" {
			groups[name] = true
		}
	}
	models := make(map[string]bool)
	for _, name := range candidate.GetModels() {
		models[name] = true
	}
	for name := range route.groups {
		if !groups[name] {
			return fmt.Errorf("%w (channel %d)", ErrRequiredChannelRoute, current.Id)
		}
	}
	for name := range route.models {
		if !models[name] {
			return fmt.Errorf("%w (channel %d)", ErrRequiredChannelRoute, current.Id)
		}
	}
	if route.private && len(groups) != 1 {
		return fmt.Errorf("%w (channel %d)", ErrRequiredChannelRoute, current.Id)
	}
	if !route.private {
		for name := range p.privateNames {
			if groups[name] {
				return fmt.Errorf("%w (channel %d)", ErrRequiredChannelRoute, current.Id)
			}
		}
	}
	return nil
}

// Lock all declared rows, including rows currently outside the selected tag or
// status. Otherwise a concurrent status/tag change could enter a bulk DELETE
// after validation. The small declaration never causes a full channel scan.
func writeRequiredChannelConfiguration(db *gorm.DB, selects func(*Channel) bool, change func(*Channel), deleting bool, write func(*gorm.DB) error) error {
	p, err := loadRequiredChannelPolicy()
	if err != nil {
		return err
	}
	if p == nil {
		return write(db)
	}
	ids := make([]int, 0, len(p.routes))
	for id := range p.routes {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return db.Transaction(func(tx *gorm.DB) error {
		var rows []Channel
		if err := lockForUpdate(tx.Model(&Channel{}).Select("id", "type", "status", "models", commonGroupCol, "tag").Where("id IN ?", ids).Order("id")).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if !selects(&row) {
				continue
			}
			if deleting {
				return fmt.Errorf("%w (channel %d)", ErrRequiredChannelRoute, row.Id)
			}
			candidate := row
			change(&candidate)
			if err := p.validate(&row, &candidate); err != nil {
				return err
			}
		}
		return write(tx)
	})
}

func writeRequiredChannel(channel *Channel, save bool) error {
	if err := common.ValidateExclusiveChannelGroups(channel.Group); err != nil {
		return err
	}
	return writeRequiredChannelConfiguration(DB, func(row *Channel) bool { return row.Id == channel.Id }, func(row *Channel) {
		if save || channel.Type != 0 {
			row.Type = channel.Type
		}
		if save || channel.Status != 0 {
			row.Status = channel.Status
		}
		if save || channel.Group != "" {
			row.Group = channel.Group
		}
		if save || channel.Models != "" {
			row.Models = channel.Models
		}
	}, false, func(tx *gorm.DB) error {
		if save {
			return tx.Save(channel).Error
		}
		return tx.Model(channel).Updates(channel).Error
	})
}

func rejectRequiredChannelIDs(ids []int) error {
	p, err := loadRequiredChannelPolicy()
	if err != nil || p == nil {
		return err
	}
	for _, id := range ids {
		if _, required := p.routes[id]; required {
			return fmt.Errorf("%w (channel %d)", ErrRequiredChannelRoute, id)
		}
	}
	return nil
}

func allowRequiredChannelRuntimeStatus(id, status int) bool {
	if status != common.ChannelStatusManuallyDisabled {
		return true
	}
	return rejectRequiredChannelIDs([]int{id}) == nil
}

// ValidateChannelStatusWrite gives admin APIs an explicit, all-or-none error
// before the runtime bool-returning circuit-breaker path changes any members.
func ValidateChannelStatusWrite(ids []int, status int) error {
	selected := make(map[int]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	return writeRequiredChannelConfiguration(DB, func(row *Channel) bool { return selected[row.Id] }, func(row *Channel) { row.Status = status }, false, func(_ *gorm.DB) error { return nil })
}

func editRequiredChannelsByTag(tag string, patch Channel) error {
	return writeRequiredChannelConfiguration(DB, func(row *Channel) bool { return row.GetTag() == tag }, func(row *Channel) {
		if patch.Group != "" {
			row.Group = patch.Group
		}
		if patch.Models != "" {
			row.Models = patch.Models
		}
	}, false, func(tx *gorm.DB) error { return tx.Model(&Channel{}).Where("tag = ?", tag).Updates(patch).Error })
}

func setRequiredChannelsStatusByTag(tag string, status int) error {
	return writeRequiredChannelConfiguration(DB, func(row *Channel) bool { return row.GetTag() == tag }, func(row *Channel) { row.Status = status }, false,
		func(tx *gorm.DB) error {
			return tx.Model(&Channel{}).Where("tag = ?", tag).Update("status", status).Error
		})
}

func deleteRequiredChannelsByStatus(status int64, allDisabled bool) (int64, error) {
	var affected int64
	err := writeRequiredChannelConfiguration(DB, func(row *Channel) bool {
		if allDisabled {
			return row.Status == common.ChannelStatusAutoDisabled || row.Status == common.ChannelStatusManuallyDisabled
		}
		return int64(row.Status) == status
	}, nil, true, func(tx *gorm.DB) error {
		query := tx.Where("status = ?", status)
		if allDisabled {
			query = tx.Where("status = ? or status = ?", common.ChannelStatusAutoDisabled, common.ChannelStatusManuallyDisabled)
		}
		result := query.Delete(&Channel{})
		affected = result.RowsAffected
		return result.Error
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func writeRequiredChannelCatalog(channel *Channel, updates map[string]interface{}, updateModels bool) error {
	write := func(db *gorm.DB) error {
		return writeRequiredChannelConfiguration(db, func(row *Channel) bool { return row.Id == channel.Id }, func(row *Channel) {
			if models, ok := updates["models"].(string); ok {
				row.Models = models
			}
		}, false, func(tx *gorm.DB) error {
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
				return err
			}
			if updateModels {
				return channel.UpdateAbilities(tx)
			}
			return nil
		})
	}
	if updateModels {
		return DB.Transaction(write)
	}
	return write(DB)
}

func validateRequiredChannelCreation(channel *Channel) error {
	p, err := loadRequiredChannelPolicy()
	if err != nil || p == nil {
		return err
	}
	if _, required := p.routes[channel.Id]; !required {
		return nil
	}
	candidate := *channel
	if candidate.Status == 0 {
		candidate.Status = common.ChannelStatusEnabled
	}
	return p.validate(&Channel{Id: channel.Id, Status: common.ChannelStatusEnabled}, &candidate)
}
