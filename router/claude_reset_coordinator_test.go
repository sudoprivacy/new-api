// sudoapi: Coordinate Claude resets across native channels.

package router

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCoordinatedClaudeResetThroughRealRelay(t *testing.T) {
	for _, test := range []struct {
		name     string
		healthy  bool
		unknown  bool
		wantCode int
		claims   int32
	}{
		{name: "existing_capacity_before_spending", healthy: true, wantCode: 200},
		{name: "unknown_evidence_preserves_credits", unknown: true, wantCode: 503},
		{name: "earliest_expiry_then_single_replay", wantCode: 200, claims: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			setupRelayRouterTestDB(t)
			require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Log{}, &model.Option{},
				&model.UserSubscription{}, &model.SubscriptionPlan{}, &model.ClaudeResetPending{}))
			originalMemory, originalRetry := common.MemoryCacheEnabled, common.RetryTimes
			common.MemoryCacheEnabled, common.RetryTimes = true, 3
			t.Cleanup(func() { common.MemoryCacheEnabled, common.RetryTimes = originalMemory, originalRetry })
			originalRatios := ratio_setting.ModelRatio2JSONString()
			require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"claude-sonnet-4-6":1.5}`))
			t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalRatios)) })
			const token = "test-reset-coordinator-token-only-12345678"
			var probes, claims, replays atomic.Int32
			var merlionRecovered atomic.Bool
			now := time.Now().UTC()
			makePeer := func(merlion bool) *httptest.Server {
				return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/v1/messages" {
						if merlion && (test.healthy || merlionRecovered.Load()) {
							if merlionRecovered.Load() {
								replays.Add(1)
							}
							_, _ = io.WriteString(w, `{"id":"msg_fixture","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"RECOVERED"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
							return
						}
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = io.WriteString(w, `{"type":"error","error":{"type":"api_error","message":"No available accounts: no available accounts"}}`)
						return
					}
					if r.Header.Get("X-Sudoapi-Reset-Coordinator") != token || r.Header.Get("X-Api-Key") != "test-native-key" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					if merlion && (r.Header.Get("CF-Access-Client-Id") != "edge-service-id" ||
						r.Header.Get("CF-Access-Client-Secret") != "edge-service-secret" ||
						r.Header.Get("X-Unrelated-Header") != "" || r.Header.Get("Authorization") != "") {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					switch r.URL.Path {
					case "/v1/sub2api/claude-reset/evidence":
						probes.Add(1)
						if merlion && test.unknown {
							_, _ = io.WriteString(w, `{"code":0,"data":{"state":"unknown"}}`)
							return
						}
						expiry := now.Add(2 * time.Hour)
						if merlion {
							expiry = now.Add(time.Hour)
						}
						_, _ = fmt.Fprintf(w, `{"code":0,"data":{"state":"exhausted","valid_until":%q,"credit_expiry":%q,"offer":%q}}`,
							now.Add(30*time.Second).Format(time.RFC3339Nano), expiry.Format(time.RFC3339Nano), strings.Repeat("a", 64))
					case "/v1/sub2api/claude-reset/claim":
						claims.Add(1)
						if !merlion {
							w.WriteHeader(http.StatusConflict)
							return
						}
						merlionRecovered.Store(true)
						_, _ = io.WriteString(w, `{"code":0,"data":{"state":"recovered"}}`)
					default:
						w.WriteHeader(http.StatusNotFound)
					}
				}))
			}
			fuji, merlion := makePeer(false), makePeer(true)
			defer fuji.Close()
			defer merlion.Close()
			roots := x509.NewCertPool()
			roots.AddCert(fuji.Certificate())
			roots.AddCert(merlion.Certificate())
			originalTransport := http.DefaultTransport
			transport := originalTransport.(*http.Transport).Clone()
			transport.TLSClientConfig = &tls.Config{RootCAs: roots}
			http.DefaultTransport = transport
			service.InitHttpClient()
			t.Cleanup(func() {
				http.DefaultTransport = originalTransport
				transport.CloseIdleConnections()
				service.InitHttpClient()
			})
			t.Setenv("SUDOAPI_RESET_COORDINATOR_PEERS", fmt.Sprintf(`{"110":{"origin":%q,"token":%q},"115":{"origin":%q,"token":%q}}`,
				fuji.URL, token, merlion.URL, token))
			primary, fallback := int64(200), int64(100)
			edgeHeaders := `{"CF-Access-Client-Id":"edge-service-id","cf-access-client-secret":"edge-service-secret","X-Unrelated-Header":"ignore","Authorization":"ignore"}`
			require.NoError(t, model.BatchInsertChannels([]model.Channel{
				{Id: 110, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "test-native-key", BaseURL: &fuji.URL, Models: "claude-sonnet-4-6", Group: "default", Priority: &primary},
				{Id: 115, Type: constant.ChannelTypeAnthropic, Status: common.ChannelStatusEnabled, Key: "test-native-key", BaseURL: &merlion.URL, Models: "claude-sonnet-4-6", Group: "default", Priority: &fallback, HeaderOverride: &edgeHeaders},
			}))
			model.InitChannelCache()
			user := model.User{Username: "coordinated-reset-fixture", Status: common.UserStatusEnabled, Group: "default", Quota: 10000000}
			require.NoError(t, model.DB.Create(&user).Error)
			require.NoError(t, model.DB.Create(&model.Token{UserId: user.Id, Key: "coordinatedresetfixturekey", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
			engine := gin.New()
			SetRelayRouter(engine)
			body := `{"model":"claude-sonnet-4-6","max_tokens":8,"messages":[{"role":"user","content":"Fixture task"}]}`
			request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("x-api-key", "coordinatedresetfixturekey")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, test.wantCode, response.Code, response.Body.String())
			require.Equal(t, test.claims, claims.Load())
			require.Equal(t, test.claims, replays.Load())
			if test.healthy {
				require.Zero(t, probes.Load())
			} else {
				require.Positive(t, probes.Load())
			}
		})
	}
}
