// sudoapi: Protect declared gateway channel routes.

package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requiredChannelFixture(t *testing.T) (Channel, Channel, Channel) {
	t.Helper()
	setupChannelStatusTest(t)
	t.Setenv("REQUIRED_CHANNEL_POLICY_FILE", "")
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "owner-private")
	shared := Channel{Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Name: "shared", Key: "fixture", Group: "customer,client-pool", Models: "required-model,optional-model", Tag: common.GetPointer("family")}
	private := Channel{Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Name: "private", Key: "fixture", Group: "owner-private", Models: "required-model,optional-model", Tag: common.GetPointer("family")}
	other := Channel{Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Name: "optional", Key: "fixture", Group: "customer", Models: "required-model", Tag: common.GetPointer("optional")}
	for _, c := range []*Channel{&shared, &private, &other} {
		require.NoError(t, c.Insert())
	}
	body, err := common.Marshal(map[string]any{"gateway": "https://gateway.example.test", "private_groups": []any{map[string]any{"name": "owner-private", "allowed_user_ids": []int{42}, "required_channel_ids": []int{private.Id}, "required_models": []string{"required-model"}}}, "shared_channels": []any{map[string]any{"channel_id": shared.Id, "required_groups": []string{"customer", "client-pool"}, "required_models": []string{"required-model"}}}})
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "channel-routing-policy.json")
	require.NoError(t, os.WriteFile(p, body, 0600))
	t.Setenv("REQUIRED_CHANNEL_POLICY_FILE", p)
	return shared, private, other
}

func TestRequiredChannelManagedUpstreamCannotBypassPublicationProxy(t *testing.T) {
	shared, _, _ := requiredChannelFixture(t)
	path := os.Getenv("REQUIRED_CHANNEL_POLICY_FILE")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var policy map[string]any
	require.NoError(t, common.Unmarshal(body, &policy))
	policy["upstream_routes"] = []any{map[string]any{"channel_ids": []int{shared.Id}, "base_url": "https://native.example.test"}}
	body, err = common.Marshal(policy)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0600))
	shared.BaseURL = common.GetPointer("https://native.example.test")
	require.NoError(t, shared.Update(), "explicit migration to the declared frontend remains possible")
	for _, base := range []string{"http://native.example.test:8080", "https://native.example.test:18080", "https://other.example.test"} {
		patch := shared
		patch.BaseURL = common.GetPointer(base)
		require.ErrorIs(t, patch.Update(), ErrRequiredChannelRoute)
		stored, err := GetChannelById(shared.Id, true)
		require.NoError(t, err)
		require.Equal(t, "https://native.example.test", *stored.BaseURL)
	}
	shared.Name = "ordinary metadata retained"
	require.NoError(t, shared.Update())
}

func TestRequiredChannelConfigurationWritesCannotDropRoutes(t *testing.T) {
	shared, private, _ := requiredChannelFixture(t)
	for name, change := range map[string]func(*Channel){
		"missing member group":                   func(c *Channel) { c.Group = "customer" },
		"whitespace is not a served model alias": func(c *Channel) { c.Models = " required-model,optional-model" },
		"missing model":                          func(c *Channel) { c.Models = "optional-model" },
		"disabled":                               func(c *Channel) { c.Status = common.ChannelStatusManuallyDisabled },
		"pretend health disable":                 func(c *Channel) { c.Status = common.ChannelStatusAutoDisabled },
		"different protocol":                     func(c *Channel) { c.Type = constant.ChannelTypeOpenAI },
	} {
		t.Run(name, func(t *testing.T) {
			patch := shared
			change(&patch)
			require.ErrorIs(t, patch.Update(), ErrRequiredChannelRoute)
			stored, err := GetChannelById(shared.Id, true)
			require.NoError(t, err)
			assert.Equal(t, shared.Group, stored.Group)
			assert.Equal(t, shared.Models, stored.Models)
			assert.Equal(t, shared.Status, stored.Status)
		})
	}
	patch := private
	patch.Group = "customer"
	require.ErrorIs(t, patch.Update(), ErrRequiredChannelRoute)
	shared.Name = "metadata is editable"
	require.NoError(t, shared.Update())
	shared.Models = "required-model,new-model"
	require.NoError(t, shared.Update())
	stored, err := GetChannelById(shared.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "metadata is editable", stored.Name)
	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", shared.Id).Find(&abilities).Error)
	assert.Len(t, abilities, 4)
}

func TestRequiredChannelDeleteAndTagOperationsAreAtomic(t *testing.T) {
	shared, private, other := requiredChannelFixture(t)
	require.ErrorIs(t, shared.Delete(), ErrRequiredChannelRoute)
	_, err := BatchDeleteChannels([]int{other.Id, private.Id})
	require.ErrorIs(t, err, ErrRequiredChannelRoute)
	_, err = GetChannelById(other.Id, true)
	require.NoError(t, err, "batch must not partially delete the unprotected member")
	require.ErrorIs(t, DisableChannelByTag("family"), ErrRequiredChannelRoute)
	require.ErrorIs(t, EditChannelByTag("family", common.GetPointer("must-not-save"), nil, common.GetPointer("optional-model"), nil, nil, nil, nil, nil), ErrRequiredChannelRoute)
	for _, id := range []int{shared.Id, private.Id} {
		stored, err := GetChannelById(id, true)
		require.NoError(t, err)
		assert.Equal(t, "family", stored.GetTag())
		assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	}
	require.NoError(t, EditChannelByTag("family", nil, nil, nil, nil, common.GetPointer[int64](197), nil, nil, nil))
	require.NoError(t, other.Delete())
}

func TestRequiredChannelRetainsHealthCircuitBreakerAndGuardsDisabledCleanup(t *testing.T) {
	shared, _, other := requiredChannelFixture(t)
	require.False(t, UpdateChannelStatus(shared.Id, "fixture", common.ChannelStatusManuallyDisabled, "operator disable"))
	require.True(t, UpdateChannelStatus(shared.Id, "fixture", common.ChannelStatusAutoDisabled, "genuine provider failure"))
	stored, err := GetChannelById(shared.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", other.Id).Update("status", common.ChannelStatusAutoDisabled).Error)
	_, err = DeleteDisabledChannel()
	require.ErrorIs(t, err, ErrRequiredChannelRoute)
	_, err = GetChannelById(other.Id, true)
	require.NoError(t, err, "cleanup rejection must preserve every selected row")
	_, err = DeleteChannelByStatus(int64(common.ChannelStatusAutoDisabled))
	require.ErrorIs(t, err, ErrRequiredChannelRoute)
	stored.Name = "editable during a real provider outage"
	require.NoError(t, stored.Update())
	require.NoError(t, EnableChannelByTag("family"))
}

func TestRequiredChannelCatalogAndFullSaveCannotBypassProtection(t *testing.T) {
	shared, _, _ := requiredChannelFixture(t)
	shared.Models = "optional-model"
	require.ErrorIs(t, PersistDiscoveredChannelModels(&shared, true), ErrRequiredChannelRoute)
	stored, err := GetChannelById(shared.Id, true)
	require.NoError(t, err)
	assert.Contains(t, stored.Models, "required-model")
	shared = *stored
	shared.Group = "customer"
	require.ErrorIs(t, shared.Save(), ErrRequiredChannelRoute)
	shared = *stored
	shared.Group = "customer,owner-private"
	require.ErrorIs(t, shared.Save(), common.ErrMixedExclusiveChannelGroups)
}

func TestRequiredChannelInvalidPolicyFailsClosedWithoutTouchingData(t *testing.T) {
	shared, _, _ := requiredChannelFixture(t)
	t.Setenv("REQUIRED_CHANNEL_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	shared.Name = "must not save"
	require.ErrorIs(t, shared.Update(), ErrRequiredChannelPolicy)
	_, err := BatchDeleteChannels([]int{shared.Id})
	require.ErrorIs(t, err, ErrRequiredChannelPolicy)
	require.ErrorIs(t, ValidateChannelStatusWrite([]int{shared.Id}, common.ChannelStatusEnabled), ErrRequiredChannelPolicy)
	stored, err := GetChannelById(shared.Id, true)
	require.NoError(t, err)
	assert.Equal(t, "shared", stored.Name)
}
