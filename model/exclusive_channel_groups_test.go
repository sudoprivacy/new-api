// sudoapi: Enforce explicit token groups for private channels.

package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExclusiveChannelBatchCreateRollsBackInvalidGroups(t *testing.T) {
	setupChannelStatusTest(t)
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "owner-private")
	channels := []Channel{
		{Type: constant.ChannelTypeAnthropic, Name: "shared", Key: "fixture-key", Models: "fixture-model", Group: "customer"},
		{Type: constant.ChannelTypeAnthropic, Name: "mixed", Key: "fixture-key", Models: "fixture-model", Group: "customer,owner-private"},
	}
	require.ErrorIs(t, BatchInsertChannels(channels), common.ErrMixedExclusiveChannelGroups)
	var count int64
	require.NoError(t, DB.Model(&Channel{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, DB.Model(&Ability{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestExclusiveChannelUpdateRejectsBeforeChangingRowsOrAbilities(t *testing.T) {
	setupChannelStatusTest(t)
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "owner-private")
	original := Channel{Type: constant.ChannelTypeAnthropic, Name: "shared", Key: "fixture-key", Models: "fixture-model", Group: "customer", Priority: common.GetPointer[int64](198)}
	require.NoError(t, original.Insert())
	patch := original
	patch.Name = "must not persist"
	patch.Group = "customer,owner-private"
	require.ErrorIs(t, patch.Update(), common.ErrMixedExclusiveChannelGroups)
	stored, err := GetChannelById(original.Id, true)
	require.NoError(t, err)
	assert.Equal(t, original.Group, stored.Group)
	assert.Equal(t, original.Name, stored.Name)
	assert.Equal(t, original.Key, stored.Key)
	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", original.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	assert.Equal(t, "customer", abilities[0].Group)
	stored.Group = "owner-private"
	require.NoError(t, stored.Update())
	abilities = nil
	require.NoError(t, DB.Where("channel_id = ?", original.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	assert.Equal(t, "owner-private", abilities[0].Group)
}

func TestExclusiveChannelTagEditRejectsAtomicallyAndAllowsUnrelatedMetadata(t *testing.T) {
	setupChannelStatusTest(t)
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "owner-private")
	for _, group := range []string{"customer", "owner-private"} {
		channel := Channel{Type: constant.ChannelTypeAnthropic, Name: group, Key: "fixture-key", Models: "fixture-model", Group: group, Tag: common.GetPointer("family"), Priority: common.GetPointer[int64](198)}
		require.NoError(t, channel.Insert())
	}
	require.ErrorIs(t, EditChannelByTag("family", common.GetPointer("must-not-change"), nil, nil, common.GetPointer("customer,owner-private"), common.GetPointer[int64](7), nil, nil, nil), common.ErrMixedExclusiveChannelGroups)
	var rows []Channel
	require.NoError(t, DB.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Equal(t, "family", row.GetTag())
		assert.Equal(t, int64(198), *row.Priority)
		assert.Equal(t, row.Name, row.Group)
	}
	require.NoError(t, EditChannelByTag("family", nil, nil, nil, nil, common.GetPointer[int64](197), nil, nil, nil))
	rows = nil
	require.NoError(t, DB.Order("id").Find(&rows).Error)
	for _, row := range rows {
		assert.Equal(t, row.Name, row.Group)
		assert.Equal(t, int64(197), *row.Priority)
	}
}
