// sudoapi: Preserve committed relay response boundaries.

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
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real HTTP requests traverse auth, channel selection, the Claude adaptor,
// scanner, retry policy, billing and error delivery against a private SQLite DB.
func TestClaudeRetryPreservesHTTPResponseBoundary(t *testing.T) {
	for _, committed := range []bool{true, false} {
		t.Run(fmt.Sprintf("committed_%t", committed), func(t *testing.T) {
			setupRelayRouterTestDB(t)
			require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Log{}, &model.Option{}, &model.UserSubscription{}, &model.SubscriptionPlan{}))
			originalRatios := ratio_setting.ModelRatio2JSONString()
			var ratios map[string]float64
			require.NoError(t, common.UnmarshalJsonStr(originalRatios, &ratios))
			ratios["claude-sonnet-4-6"] = 1.5
			configuredRatios, err := common.Marshal(ratios)
			require.NoError(t, err)
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(configuredRatios)))
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalRatios)) })
			originalRetry := common.RetryTimes
			originalTimeout := constant.StreamingTimeout
			originalConsumeLogging := common.LogConsumeEnabled
			common.RetryTimes = 2
			constant.StreamingTimeout = 30
			common.LogConsumeEnabled = true
			t.Cleanup(func() {
				common.RetryTimes = originalRetry
				constant.StreamingTimeout = originalTimeout
				common.LogConsumeEnabled = originalConsumeLogging
			})
			var attempts atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if attempts.Add(1) == 1 {
					if !committed {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadGateway)
						_, _ = io.WriteString(w, `{"error":{"type":"api_error","message":"Upstream request failed"}}`)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "event: message_start\ndata: "+`{"type":"message_start","message":{"id":"msg_attempt_a","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":2,"output_tokens":0,"cache_creation_input_tokens":5235,"cache_read_input_tokens":124730}}}`+"\n\n")
					w.(http.Flusher).Flush()
					_, _ = io.WriteString(w, "event: error\ndata: "+`{"type":"error","error":{"type":"api_error","message":"Upstream request failed"}}`+"\n\n")
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "event: message_start\ndata: "+`{"type":"message_start","message":{"id":"msg_attempt_b","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":2,"output_tokens":0,"cache_creation_input_tokens":0,"cache_read_input_tokens":129965}}}`+"\n\n")
				_, _ = io.WriteString(w, "event: content_block_start\ndata: "+`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`+"\n\nevent: content_block_delta\ndata: "+`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"RECOVERED_RESPONSE"}}`+"\n\nevent: content_block_stop\ndata: "+`{"type":"content_block_stop","index":0}`+"\n\n")
				_, _ = io.WriteString(w, "event: message_delta\ndata: "+`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1569}}`+"\n\nevent: message_stop\ndata: "+`{"type":"message_stop"}`+"\n\n")
			}))
			defer upstream.Close()
			user := model.User{Username: "retry-boundary-user", Status: common.UserStatusEnabled, Group: "default", Quota: 10000000}
			require.NoError(t, model.DB.Create(&user).Error)
			require.NoError(t, model.DB.Create(&model.Token{
				UserId: user.Id, Key: "retryboundarytestkey", Status: common.TokenStatusEnabled,
				ExpiredTime: -1, UnlimitedQuota: true,
			}).Error)
			channels := []model.Channel{{Name: "retry-boundary-fixture", Type: constant.ChannelTypeAnthropic,
				Status: common.ChannelStatusEnabled, Key: "private-fixture-upstream-key",
				BaseURL: &upstream.URL, Models: "claude-sonnet-4-6", Group: "default"}}
			require.NoError(t, model.BatchInsertChannels(channels))
			model.InitChannelCache()
			engine := gin.New()
			SetRelayRouter(engine)
			gateway := httptest.NewServer(engine)
			defer gateway.Close()
			request, err := http.NewRequest(http.MethodPost, gateway.URL+"/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-6","max_tokens":1600,"stream":true,"messages":[{"role":"user","content":"Report fixture"}]}`))
			require.NoError(t, err)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("x-api-key", "retryboundarytestkey")
			response, err := gateway.Client().Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode, "%s", body)
			var starts []string
			var errorEvents int
			var startUsage dto.ClaudeUsage
			for _, line := range strings.Split(string(body), "\n") {
				if !strings.HasPrefix(line, "data:") {
					continue
				}
				var event struct {
					Type    string `json:"type"`
					Message struct {
						ID    string          `json:"id"`
						Usage dto.ClaudeUsage `json:"usage"`
					} `json:"message"`
				}
				require.NoError(t, common.UnmarshalJsonStr(strings.TrimSpace(line[5:]), &event))
				if event.Type == "message_start" {
					starts = append(starts, event.Message.ID)
					startUsage = event.Message.Usage
				} else if event.Type == "error" {
					errorEvents++
				}
			}
			var consumeLogs []model.Log
			require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Find(&consumeLogs).Error)
			if committed {
				assert.EqualValues(t, 1, attempts.Load(), "never restart a committed model response")
				assert.Equal(t, []string{"msg_attempt_a"}, starts)
				assert.Equal(t, 1, errorEvents)
				assert.Empty(t, consumeLogs, "an interrupted stream must not become a successful consume log")
				assert.NotContains(t, string(body), "RECOVERED_RESPONSE")
			} else {
				assert.EqualValues(t, 2, attempts.Load(), "retain failover before output starts")
				assert.Equal(t, []string{"msg_attempt_b"}, starts)
				assert.Zero(t, startUsage.CacheCreationInputTokens)
				assert.Equal(t, 129965, startUsage.CacheReadInputTokens)
				assert.Zero(t, errorEvents)
				assert.Contains(t, string(body), "RECOVERED_RESPONSE")
				require.Len(t, consumeLogs, 1)
				assert.Equal(t, 2, consumeLogs[0].PromptTokens)
				assert.Equal(t, 1569, consumeLogs[0].CompletionTokens)
				var billing struct {
					CacheCreationTokens int `json:"cache_creation_tokens"`
					CacheReadTokens     int `json:"cache_tokens"`
				}
				require.NoError(t, common.UnmarshalJsonStr(consumeLogs[0].Other, &billing))
				assert.Zero(t, billing.CacheCreationTokens)
				assert.Equal(t, 129965, billing.CacheReadTokens)
			}
		})
	}
}
