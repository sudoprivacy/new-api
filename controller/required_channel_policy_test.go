// sudoapi: Protect declared gateway channel routes.

package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequiredRouteHTTPWritesRejectWithoutPartialChanges(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	t.Setenv("REQUIRED_CHANNEL_POLICY_FILE", "")
	required := model.Channel{Type: constant.ChannelTypeAnthropic, Name: "required", Key: "fixture", Status: common.ChannelStatusEnabled, Models: "required-model,optional-model", Group: "customer,client-pool", Tag: common.GetPointer("family")}
	other := model.Channel{Type: constant.ChannelTypeAnthropic, Name: "optional", Key: "fixture", Status: common.ChannelStatusEnabled, Models: "required-model", Group: "customer"}
	require.NoError(t, db.Create(&required).Error)
	require.NoError(t, db.Create(&other).Error)
	data, err := common.Marshal(map[string]any{"gateway": "https://gateway.example.test", "shared_channels": []any{map[string]any{"channel_id": required.Id, "required_groups": []string{"customer", "client-pool"}, "required_models": []string{"required-model"}}}})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "policy.json")
	require.NoError(t, os.WriteFile(path, data, 0600))
	t.Setenv("REQUIRED_CHANNEL_POLICY_FILE", path)
	for _, tc := range []struct {
		name, method, body string
		handler            gin.HandlerFunc
	}{
		{"drop group", http.MethodPut, fmt.Sprintf(`{"id":%d,"group":"customer"}`, required.Id), UpdateChannel},
		{"drop model", http.MethodPut, fmt.Sprintf(`{"id":%d,"models":"optional-model"}`, required.Id), UpdateChannel},
		{"delete", http.MethodDelete, "", DeleteChannel},
		{"batch delete", http.MethodPost, fmt.Sprintf(`{"ids":[%d,%d]}`, other.Id, required.Id), DeleteChannelBatch},
		{"disable", http.MethodPut, `{"status":2}`, UpdateChannelStatus},
		{"batch disable", http.MethodPut, fmt.Sprintf(`{"ids":[%d,%d],"status":2}`, other.Id, required.Id), BatchUpdateChannelStatus},
		{"tag disable", http.MethodPost, `{"tag":"family"}`, DisableTagChannels},
		{"tag models", http.MethodPost, `{"tag":"family","models":"optional-model"}`, EditTagChannels},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(r)
			ctx.Set("id", 1)
			ctx.Set("role", common.RoleRootUser)
			ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprint(required.Id)}}
			ctx.Request = httptest.NewRequest(tc.method, "/api/channel/", strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			tc.handler(ctx)
			var result struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(r.Body.Bytes(), &result))
			require.False(t, result.Success, r.Body.String())
			require.Contains(t, result.Message, model.ErrRequiredChannelRoute.Error())
			for _, original := range []model.Channel{required, other} {
				var stored model.Channel
				require.NoError(t, db.First(&stored, original.Id).Error)
				require.Equal(t, original.Group, stored.Group)
				require.Equal(t, original.Models, stored.Models)
				require.Equal(t, original.Status, stored.Status)
			}
		})
	}
}
