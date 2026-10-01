// sudoapi: Discover models, capabilities, and reference prices from trusted upstream catalogs.
package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/upstream_catalog"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTrustedCatalogUnseenModelReachesListingAndBilling(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	common.OptionMap = make(map[string]string)
	withSelfUseModeDisabled(t)
	withTieredBillingConfig(t, nil, nil)
	require.NoError(t, upstream_catalog.Update("{}"))
	t.Cleanup(func() { require.NoError(t, upstream_catalog.Update("{}")) })
	priced := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-key", r.Header.Get("x-api-key"))
		price := ""
		if priced {
			price = `,"reference_pricing":{"currency":"USD","unit":"per_million_tokens","input":2,"output":10,"cache_read":0.2,"cache_write_5m":2.5,"cache_write_1h":4}`
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-unseen-2040","max_input_tokens":1234567,"max_tokens":98765,"capabilities":{"image_input":{"supported":false}}` + price + `}]}`))
	}))
	defer server.Close()
	base := server.URL
	channel := &model.Channel{Name: "catalog", Type: constant.ChannelTypeAnthropic, Key: "test-key", Status: 1, BaseURL: &base, Group: "catalog-test"}
	settings := dto.ChannelOtherSettings{UpstreamModelUpdateCheckEnabled: true, UpstreamModelUpdateAutoSyncEnabled: true, UpstreamCatalogTrusted: true}
	channel.SetOtherSettings(settings)
	require.NoError(t, db.Create(channel).Error)
	list := func(limited bool) []dto.OpenAIModels {
		r := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(r)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		ctx.Set(string(constant.ContextKeyTokenGroup), "catalog-test")
		ctx.Set(string(constant.ContextKeyUserGroup), "catalog-test")
		if limited {
			ctx.Set(string(constant.ContextKeyTokenModelLimitEnabled), true)
			ctx.Set(string(constant.ContextKeyTokenModelLimit), map[string]bool{"another": true})
		}
		ListModels(ctx, constant.ChannelTypeOpenAI)
		var response listModelsResponse
		require.NoError(t, common.Unmarshal(r.Body.Bytes(), &response))
		require.True(t, response.Success)
		return response.Data
	}
	changed, added, err := checkAndPersistChannelUpstreamModelUpdates(channel, &settings, true, true)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, added)
	require.Empty(t, list(false), "unpriced model must wait for a documented price")
	priced = true
	changed, added, err = checkAndPersistChannelUpstreamModelUpdates(channel, &settings, true, true)
	require.NoError(t, err)
	require.False(t, changed, "price refresh must also run for models already registered")
	require.Zero(t, added)
	models := list(false)
	require.Len(t, models, 1)
	require.Equal(t, "claude-unseen-2040", models[0].Id)
	meta := model_setting.GetModelMetadata("claude-unseen-2040")
	require.EqualValues(t, 1234567, meta.ContextWindow)
	require.EqualValues(t, 98765, meta.MaxOutputTokens)
	require.False(t, *meta.VisionSupported)
	require.Empty(t, list(true), "explicit token restrictions must still apply")
	expr, ok := billing_setting.GetBillingExpr("claude-unseen-2040")
	require.True(t, ok)
	require.Equal(t, billing_setting.BillingModeTieredExpr, billing_setting.GetBillingMode("claude-unseen-2040"))
	cost, _, err := billingexpr.RunExprWithRequest(expr, billingexpr.TokenParams{P: 100, C: 10, CR: 1000, CC: 20, CC1h: 30}, billingexpr.RequestInput{})
	require.NoError(t, err)
	require.InDelta(t, 670, cost, 1e-8)
	// An explicit operator price wins even after a successful catalog refresh.
	oldRatios := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios)) })
	rates := ratio_setting.GetModelRatioCopy()
	rates["claude-unseen-2040"] = 7
	body, err := common.Marshal(rates)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(body)))
	require.Equal(t, billing_setting.BillingModeRatio, billing_setting.GetBillingMode("claude-unseen-2040"))
	_, ok = billing_setting.GetBillingExpr("claude-unseen-2040")
	require.False(t, ok)
}

func TestTrustedCatalogRoutingFailureRollsBackChannelList(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	channel := &model.Channel{Name: "atomic-catalog", Models: "previous", Group: "test", Status: 1}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_catalog_ability", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("routing database unavailable"))
		}
	}))
	t.Cleanup(func() { db.Callback().Create().Remove("fail_catalog_ability") })
	channel.Models = "previous,unseen"
	require.ErrorContains(t, model.PersistDiscoveredChannelModels(channel, true), "routing database unavailable")
	var persisted model.Channel
	require.NoError(t, db.First(&persisted, channel.Id).Error)
	require.Equal(t, "previous", persisted.Models)
	var abilities []model.Ability
	require.NoError(t, db.Where("channel_id = ?", channel.Id).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.Equal(t, "previous", abilities[0].Model)
}

func TestTrustedCatalogRejectsIncompletePaginationWithoutChangingSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"partial"}],"has_more":true}`))
	}))
	defer server.Close()
	base := server.URL
	_, _, err := fetchTrustedChannelCatalog(&model.Channel{Type: constant.ChannelTypeAnthropic, Key: "secret", BaseURL: &base})
	require.ErrorContains(t, err, "pagination")
}
