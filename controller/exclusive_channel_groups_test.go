// sudoapi: Enforce explicit token groups for private channels.

package controller

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExclusiveChannelHTTPUpdateRejectsMixedGroups(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "owner-private")
	channel := model.Channel{Type: constant.ChannelTypeAnthropic, Name: "shared", Key: "fixture-key", Models: "fixture-model", Group: "customer"}
	require.NoError(t, db.Create(&channel).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/channel/", strings.NewReader(fmt.Sprintf(`{"id":%d,"group":"customer,owner-private"}`, channel.Id)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(ctx)
	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	assert.False(t, result.Success)
	assert.Equal(t, common.ErrMixedExclusiveChannelGroups.Error(), result.Message)
	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, "customer", stored.Group)
	assert.Equal(t, "fixture-key", stored.Key)
}

func TestExclusiveChannelCopyRejectsLegacyMixedGroups(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "")
	origin := model.Channel{Type: constant.ChannelTypeAnthropic, Name: "legacy mixed", Key: "fixture-key", Models: "fixture-model", Group: "customer,owner-private"}
	require.NoError(t, db.Create(&origin).Error)
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "owner-private")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy/", nil)
	CopyChannel(ctx)
	assert.Contains(t, recorder.Body.String(), common.ErrMixedExclusiveChannelGroups.Error())
	var count int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
