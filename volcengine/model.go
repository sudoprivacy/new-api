// sudoapi: Volcengine ark asset.

package volcengine

import (
	"time"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/QuantumNous/new-api/model"
)

type (
	ArkAssetGroup struct {
		ID        string `gorm:"type:varchar(32);primaryKey"`
		CreatedAt time.Time
		UpdatedAt time.Time
		DeletedAt gorm.DeletedAt `gorm:"index"`

		UserID    int       `gorm:"not null;index"`
		GroupType GroupType `gorm:"not null;index"`
	}

	ArkAsset struct {
		ID        string `gorm:"type:varchar(32);primaryKey"`
		CreatedAt time.Time
		UpdatedAt time.Time
		DeletedAt gorm.DeletedAt `gorm:"index"`

		UserID  int    `gorm:"not null;index"`
		GroupID string `gorm:"not null;index"`
	}

	ArkVisualValidateSession struct {
		ID        string `gorm:"type:varchar(64);primaryKey"` // id is BytedToken
		CreatedAt time.Time
		UpdatedAt time.Time
		DeletedAt gorm.DeletedAt `gorm:"index"`

		UserID      int `gorm:"not null;index"`
		CallbackURL string

		GroupID string `gorm:"index"` // 存在 GroupID 则已经认证完成
	}
)

func (*ArkAssetGroup) TableName() string            { return "ark_asset_group" }
func (*ArkAsset) TableName() string                 { return "ark_asset" }
func (*ArkVisualValidateSession) TableName() string { return "ark_visual_validate_session" }

type GroupType string

const (
	GroupTypeAIGC         GroupType = "AIGC"
	GroupTypeLivenessFace GroupType = "LivenessFace"
)

func createAssetGroupOwnership(group *ArkAssetGroup) error {
	return model.DB.Create(group).Error
}

func createAssetOwnership(asset *ArkAsset) error {
	return model.DB.Create(asset).Error
}

func findFirstAIGCAssetGroup(userID int) (*ArkAssetGroup, error) {
	var group ArkAssetGroup
	err := model.DB.Model(&ArkAssetGroup{}).
		Where("user_id = ?", userID).
		Where("group_type = ?", GroupTypeAIGC).
		First(&group).Error
	return &group, err
}

func listOwnedAssetGroups(userID int, groupIDs []string, groupType GroupType) ([]*ArkAssetGroup, error) {
	query := model.DB.Model(&ArkAssetGroup{}).
		Where("user_id = ?", userID).
		Where("group_type = ?", lo.Ternary(
			groupType == GroupTypeLivenessFace,
			GroupTypeLivenessFace,
			GroupTypeAIGC,
		))
	if len(groupIDs) > 0 {
		query = query.Where("id IN (?)", groupIDs)
	}
	var resources []*ArkAssetGroup
	return resources, query.Find(&resources).Error
}

func isAssetGroupOwnedByUser(userID int, groupID string) (bool, error) {
	var count int64
	err := model.DB.Model(&ArkAssetGroup{}).
		Where("user_id = ?", userID).
		Where("id = ?", groupID).
		Count(&count).
		Error
	return count > 0, err
}

func deleteAssetGroupOwnership(userID int, groupID string) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("user_id = ?", userID).
			Where("group_id = ?", groupID).
			Delete(&ArkAsset{}).Error; err != nil {
			return err
		}
		return tx.
			Where("user_id = ?", userID).
			Where("id = ?", groupID).
			Delete(&ArkAssetGroup{}).Error
	})
}

func isAssetOwnedByUser(userID int, assetID string) (bool, error) {
	var count int64
	err := model.DB.Model(&ArkAsset{}).
		Where("user_id = ?", userID).
		Where("id = ?", assetID).
		Count(&count).
		Error
	return count > 0, err
}

func deleteAssetOwnership(userID int, assetID string) error {
	return model.DB.
		Where("user_id = ?", userID).
		Where("id = ?", assetID).
		Delete(&ArkAsset{}).Error
}

func createVisualValidateSession(userID int, bytedToken, callbackURL string) error {
	return model.DB.Create(&ArkVisualValidateSession{
		ID:          bytedToken,
		UserID:      userID,
		CallbackURL: callbackURL,
	}).Error
}

func completeVisualValidateSession(userID int, bytedToken, groupID string) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ArkAssetGroup{
			ID:        groupID,
			UserID:    userID,
			GroupType: GroupTypeLivenessFace,
		}).Error
		if err != nil {
			return err
		}

		var group ArkAssetGroup
		if err = tx.Where("id = ?", groupID).First(&group).Error; err != nil {
			return err
		}
		if group.UserID != userID || group.GroupType != GroupTypeLivenessFace {
			return errors.Errorf("visual validate group %s mismatch", groupID)
		}

		return tx.Model(&ArkVisualValidateSession{}).
			Where("user_id = ?", userID).
			Where("id = ?", bytedToken).
			Update("group_id", groupID).Error
	})
}

// findVisualValidateSession 在上游回调我们时没有 user id, 因此没有 user id 限制, GetVisualValidateResult 中需要验证
func findVisualValidateSession(bytedToken string) (*ArkVisualValidateSession, error) {
	var visualValidate ArkVisualValidateSession
	err := model.DB.Model(&ArkVisualValidateSession{}).
		Where("id = ?", bytedToken).
		First(&visualValidate).
		Error
	return &visualValidate, err
}
