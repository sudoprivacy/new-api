// sudoapi: Fail over explicit pre-inference capacity failures within authorized channels.

package router

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the real HTTP auth/distribution/adaptor/retry/billing path. Upstreams
// are local fixtures; these tests never log into or call a provider account.
func TestClaudeCapacityFailoverPreservesScopeAndAffinity(t *testing.T) {
	for _, memory := range []bool{true, false} {
		for _, test := range []struct {
			name              string
			pinned            int
			primaryFails      bool
			fallbackSucceeds  bool
			primaryOtherGroup bool
			skipRetry         bool
			samePriority      bool
			stream            bool
			wantStatus        int
			wantPrimary       int32
			wantFallback      int32
		}{
			{name: "lower_priority_affinity", pinned: 74, wantStatus: 200, wantPrimary: 1, wantFallback: 1},
			{name: "lower_priority_stream", pinned: 74, stream: true, wantStatus: 200, wantPrimary: 1, wantFallback: 1},
			{name: "primary_to_healthy_fallback", pinned: 42, primaryFails: true, fallbackSucceeds: true, wantStatus: 200, wantPrimary: 1, wantFallback: 1},
			{name: "healthy_affinity_stays", pinned: 74, fallbackSucceeds: true, wantStatus: 200, wantFallback: 1},
			{name: "primary_exhausted", pinned: 42, primaryFails: true, wantStatus: 503, wantPrimary: 1, wantFallback: 1},
			{name: "same_priority", pinned: 74, samePriority: true, wantStatus: 200, wantPrimary: 1, wantFallback: 1},
			{name: "unauthorized_primary", pinned: 74, primaryOtherGroup: true, wantStatus: 503, wantFallback: 1},
			{name: "explicit_no_retry", pinned: 74, skipRetry: true, wantStatus: 503, wantFallback: 1},
		} {
			t.Run(fmt.Sprintf("memory_%t/%s", memory, test.name), func(t *testing.T) {
				setupRelayRouterTestDB(t)
				require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Log{}, &model.Option{}, &model.UserSubscription{}, &model.SubscriptionPlan{}))
				originalMemory, originalRetry, originalLog := common.MemoryCacheEnabled, common.RetryTimes, common.LogConsumeEnabled
				originalStreamTimeout := constant.StreamingTimeout
				constant.StreamingTimeout = 30
				t.Cleanup(func() { constant.StreamingTimeout = originalStreamTimeout })
				common.MemoryCacheEnabled, common.RetryTimes, common.LogConsumeEnabled = memory, 3, true
				t.Cleanup(func() {
					common.MemoryCacheEnabled, common.RetryTimes, common.LogConsumeEnabled = originalMemory, originalRetry, originalLog
				})
				originalRatios := ratio_setting.ModelRatio2JSONString()
				var ratios map[string]float64
				require.NoError(t, common.UnmarshalJsonStr(originalRatios, &ratios))
				ratios["claude-sonnet-4-6"] = 1.5
				configuredRatios, err := common.Marshal(ratios)
				require.NoError(t, err)
				require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(configuredRatios)))
				t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalRatios)) })
				affinity := operation_setting.GetChannelAffinitySetting()
				originalAffinity := *affinity
				*affinity = operation_setting.ChannelAffinitySetting{Enabled: true, SwitchOnSuccess: true, DefaultTTLSeconds: 3600,
					Rules: []operation_setting.ChannelAffinityRule{{Name: "capacity-fixture", ModelRegex: []string{"^claude-"}, PathRegex: []string{"/v1/messages"},
						KeySources:         []operation_setting.ChannelAffinityKeySource{{Type: "gjson", Path: "metadata.user_id"}},
						SkipRetryOnFailure: test.skipRetry, IncludeRuleName: true, IncludeUsingGroup: true}}}
				t.Cleanup(func() { service.ClearChannelAffinityCacheAll(); *affinity = originalAffinity })
				var primaryCalls, fallbackCalls atomic.Int32
				primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					primaryCalls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if test.primaryFails {
						w.WriteHeader(503)
						_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"No available accounts: no available accounts"}}`)
						return
					}
					if test.stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "event: message_start\ndata: "+`{"type":"message_start","message":{"id":"msg_capacity_fixture","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":2,"output_tokens":0}}}`+"\n\nevent: content_block_delta\ndata: "+`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"RECOVERED"}}`+"\n\nevent: message_delta\ndata: "+`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`+"\n\nevent: message_stop\ndata: "+`{"type":"message_stop"}`+"\n\n")
						return
					}
					_, _ = io.WriteString(w, `{"id":"msg_capacity_fixture","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"RECOVERED"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
				}))
				defer primary.Close()
				fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					fallbackCalls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if test.fallbackSucceeds {
						_, _ = io.WriteString(w, `{"id":"msg_fallback_fixture","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"RECOVERED"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
						return
					}
					w.WriteHeader(503)
					_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"No available accounts: no available accounts"}}`)
				}))
				defer fallback.Close()
				primaryGroup := "default"
				if test.primaryOtherGroup {
					primaryGroup = "other-owner"
				}
				primaryPriority, fallbackPriority := int64(198), int64(99)
				if test.samePriority {
					primaryPriority = fallbackPriority
				}
				channels := []model.Channel{
					{Id: 42, Name: "primary-fixture", Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "upstream-fixture", BaseURL: &primary.URL, Models: "claude-sonnet-4-6", Group: primaryGroup, Priority: &primaryPriority},
					{Id: 74, Name: "fallback-fixture", Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "upstream-fixture", BaseURL: &fallback.URL, Models: "claude-sonnet-4-6", Group: "default", Priority: &fallbackPriority},
				}
				require.NoError(t, model.BatchInsertChannels(channels))
				model.InitChannelCache()
				user := model.User{Username: "capacity-fixture-user", Status: common.UserStatusEnabled, Group: "default", Quota: 10000000}
				require.NoError(t, model.DB.Create(&user).Error)
				require.NoError(t, model.DB.Create(&model.Token{UserId: user.Id, Key: "capacityfixturekey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
				body := `{"model":"claude-sonnet-4-6","stream":` + fmt.Sprint(test.stream) + `,"max_tokens":8,"metadata":{"user_id":"` + t.Name() + `"},"messages":[{"role":"user","content":"Fixture task"}]}`
				seed, _ := gin.CreateTestContext(httptest.NewRecorder())
				seed.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
				_, _ = service.GetPreferredChannelByAffinity(seed, "claude-sonnet-4-6", "default")
				service.RecordChannelAffinity(seed, test.pinned)
				engine := gin.New()
				SetRelayRouter(engine)
				request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("x-api-key", "capacityfixturekey")
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				require.Equal(t, test.wantStatus, response.Code, "%s", response.Body)
				assert.Equal(t, test.wantPrimary, primaryCalls.Load())
				assert.Equal(t, test.wantFallback, fallbackCalls.Load(), "an exhausted pool must not be retried repeatedly")
				verify, _ := gin.CreateTestContext(httptest.NewRecorder())
				verify.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
				bound, found := service.GetPreferredChannelByAffinity(verify, "claude-sonnet-4-6", "default")
				var consumes []model.Log
				require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Find(&consumes).Error)
				if test.wantStatus == 200 {
					assert.Contains(t, response.Body.String(), "RECOVERED")
					assert.True(t, found)
					winner := 42
					if test.fallbackSucceeds {
						winner = 74
					}
					assert.Equal(t, winner, bound, "subsequent turns must retain the successful pool")
					require.Len(t, consumes, 1, "only the successful inference can be billed")
					assert.Equal(t, winner, consumes[0].ChannelId)
				} else {
					assert.Empty(t, consumes)
					if test.skipRetry {
						assert.True(t, found)
						assert.Equal(t, 74, bound)
					} else {
						assert.False(t, found)
					}
				}
			})
		}
	}
}
