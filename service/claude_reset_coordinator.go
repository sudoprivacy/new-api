// sudoapi: Coordinate Claude resets across native channels.

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
)

const claudeResetPeerEnv = "SUDOAPI_RESET_COORDINATOR_PEERS"

type claudeResetPeer struct {
	Origin string `json:"origin"`
	Token  string `json:"token"`
}

type claudeResetRequest struct {
	Demand      string     `json:"demand"`
	Model       string     `json:"model"`
	Offer       string     `json:"offer,omitempty"`
	ValidUntil  *time.Time `json:"valid_until,omitempty"`
	ApplyBefore *time.Time `json:"apply_before,omitempty"`
}

type claudeResetProof struct {
	State        string     `json:"state"`
	Offer        string     `json:"offer"`
	ValidUntil   *time.Time `json:"valid_until"`
	CreditExpiry *time.Time `json:"credit_expiry"`
}

type claudeResetRoute struct {
	channel *model.Channel
	peer    claudeResetPeer
}

func resetChannelBinding(origin, key string) string {
	sum := sha256.Sum256([]byte(strings.TrimRight(origin, "/") + "\x00" + key))
	return hex.EncodeToString(sum[:])
}

func (r claudeResetRoute) binding() string {
	return resetChannelBinding(r.peer.Origin, r.channel.Key)
}

func newClaudeResetRoute(channel *model.Channel, peer claudeResetPeer) (claudeResetRoute, bool) {
	origin, err := url.Parse(peer.Origin)
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil ||
		origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" || origin.Path != "" || len(peer.Token) < 32 ||
		channel == nil || channel.Status != common.ChannelStatusEnabled || channel.Type != constant.ChannelTypeAnthropic ||
		channel.ChannelInfo.IsMultiKey || channel.Key == "" ||
		strings.TrimRight(channel.GetBaseURL(), "/") != peer.Origin ||
		channel.GetModelMapping() != "" && channel.GetModelMapping() != "{}" ||
		channel.ParamOverride != nil && *channel.ParamOverride != "" && *channel.ParamOverride != "{}" {
		return claudeResetRoute{}, false
	}
	return claudeResetRoute{channel: channel, peer: peer}, true
}

func (r claudeResetRoute) request(ctx context.Context, action string, input claudeResetRequest) claudeResetProof {
	body, err := common.Marshal(input)
	if err != nil {
		return claudeResetProof{}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.peer.Origin+"/v1/sub2api/claude-reset/"+action, bytes.NewReader(body))
	if err != nil {
		return claudeResetProof{}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Api-Key", r.channel.Key)
	request.Header.Set("X-Sudoapi-Reset-Coordinator", r.peer.Token)
	// Operator-pinned HTTPS origins use the gateway trust store. Keep the
	// coordinator credential on that origin even if a server redirects.
	client := http.Client{
		Timeout: 25 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(request)
	if err != nil {
		return claudeResetProof{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return claudeResetProof{}
	}
	var result struct {
		Code int              `json:"code"`
		Data claudeResetProof `json:"data"`
	}
	if common.DecodeJson(io.LimitReader(resp.Body, 4096), &result) != nil || result.Code != 0 {
		return claudeResetProof{}
	}
	return result.Data
}

func (r claudeResetRoute) matchesAttempt(param *RetryParam) bool {
	channel, err := model.GetChannelById(r.channel.Id, true)
	if err != nil {
		return false
	}
	live, ok := newClaudeResetRoute(channel, r.peer)
	if !ok || live.binding() != param.coordinatedBindings[r.channel.Id] {
		return false
	}
	others := slices.DeleteFunc(slices.Clone(param.ExhaustedChannels), func(id int) bool { return id == r.channel.Id })
	admitted, err := model.GetRandomSatisfiedChannel(param.TokenGroup, param.ModelName, 0, param.RequestPath, others...)
	return err == nil && admitted != nil && admitted.Id == r.channel.Id
}

func (p *RetryParam) allChannelsRejected() bool {
	remaining, err := model.GetRandomSatisfiedChannel(p.TokenGroup, p.ModelName, 0, p.RequestPath, p.ExhaustedChannels...)
	return err == nil && remaining == nil
}

func resolvePendingClaudeReset(ctx context.Context, peers map[string]claudeResetPeer) bool {
	pending, err := model.LoadClaudeResetPending(ctx)
	if err != nil || pending == nil {
		return err == nil
	}
	if pending.ChannelID == 0 {
		expired, err := model.ExpireClaudeResetPlan(ctx, pending.Demand)
		return err == nil && expired
	}
	channel, err := model.GetChannelById(pending.ChannelID, true)
	if err != nil {
		return false
	}
	route, ok := newClaudeResetRoute(channel, peers[strconv.Itoa(pending.ChannelID)])
	if !ok || route.binding() != pending.PeerBinding {
		return false
	}
	var input claudeResetRequest
	if common.UnmarshalJsonStr(pending.Request, &input) != nil || input.Demand != pending.Demand {
		return false
	}
	state := route.request(ctx, "status", input).State
	return (state == "recovered" || state == "not_spent") && model.SettleClaudeReset(ctx, pending.Demand) == nil
}

// TryCoordinatedClaudeReset runs after the ordinary relay's retry loop. It
// returns the only channel eligible for one replay after confirmed recovery.
func TryCoordinatedClaudeReset(param *RetryParam) int {
	c := param.Ctx
	raw := os.Getenv(claudeResetPeerEnv)
	if raw == "" || param.NonCapacityFailure || len(param.ExhaustedChannels) == 0 ||
		c.Writer.Written() || ShouldSkipRetryAfterChannelAffinityFailure(c) ||
		param.TokenGroup == "auto" || param.TokenGroup == "" || param.ModelName == "" ||
		(param.RequestPath != "/v1/messages" && param.RequestPath != "/v1/chat/completions") {
		return 0
	}
	if _, pinned := c.Get("specific_channel_id"); pinned {
		return 0
	}
	if !param.allChannelsRejected() {
		return 0
	}
	var peers map[string]claudeResetPeer
	if common.UnmarshalJsonStr(raw, &peers) != nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 55*time.Second)
	defer cancel()
	if !resolvePendingClaudeReset(ctx, peers) {
		return 0
	}
	demand := claudeResetRequest{Demand: uuid.NewString(), Model: param.ModelName}
	planDeadline, _ := ctx.Deadline()
	reserved, err := model.FenceClaudeReset(ctx, model.ClaudeResetPending{Demand: demand.Demand, PlanBefore: planDeadline.UnixNano()})
	if err != nil || !reserved {
		return 0
	}
	fencedClaim := false
	defer func() {
		if !fencedClaim {
			settle, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer done()
			_ = model.SettleClaudeReset(settle, demand.Demand)
		}
	}()
	routes := make([]claudeResetRoute, 0, len(param.ExhaustedChannels))
	for _, id := range param.ExhaustedChannels {
		channel, err := model.GetChannelById(id, true)
		if err != nil {
			return 0
		}
		route, ok := newClaudeResetRoute(channel, peers[strconv.Itoa(id)])
		if !ok || route.binding() != param.coordinatedBindings[id] {
			return 0
		}
		routes = append(routes, route)
	}
	chosen, proof, deadline := selectClaudeResetOffer(ctx, demand, routes, claudeResetRoute.request)
	if chosen.channel == nil || !param.allChannelsRejected() {
		return 0
	}
	for _, route := range routes {
		if !route.matchesAttempt(param) {
			return 0
		}
	}
	if ctx.Err() != nil || !time.Now().Before(deadline) || c.Writer.Written() {
		return 0
	}
	demand.Offer, demand.ValidUntil, demand.ApplyBefore = proof.Offer, proof.ValidUntil, &deadline
	saved, err := common.Marshal(demand)
	if err != nil {
		return 0
	}
	acquired, err := model.AuthorizeClaudeResetClaim(ctx, model.ClaudeResetPending{
		Demand: demand.Demand, ChannelID: chosen.channel.Id, PeerBinding: chosen.binding(), Request: string(saved),
	})
	// If the SQL response is lost, retain the fence. The same peer can still
	// prove no attempt occurred once the native acceptance deadline expires.
	fencedClaim = acquired || err != nil
	if err != nil || !acquired {
		return 0
	}
	state := chosen.request(ctx, "claim", demand).State
	if state != "recovered" && state != "not_spent" {
		return 0
	}
	settle, done := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer done()
	if model.SettleClaudeReset(settle, demand.Demand) != nil || state != "recovered" || ctx.Err() != nil {
		return 0
	}
	return chosen.channel.Id
}

func selectClaudeResetOffer(ctx context.Context, demand claudeResetRequest, routes []claudeResetRoute, query func(claudeResetRoute, context.Context, string, claudeResetRequest) claudeResetProof) (claudeResetRoute, claudeResetProof, time.Time) {
	type observed struct {
		route claudeResetRoute
		proof claudeResetProof
	}
	evidence := make(chan observed, len(routes))
	for _, route := range routes {
		go func() { evidence <- observed{route: route, proof: query(route, ctx, "evidence", demand)} }()
	}
	var chosen claudeResetRoute
	var offer claudeResetProof
	var deadline time.Time
	for range routes {
		select {
		case <-ctx.Done():
			return claudeResetRoute{}, claudeResetProof{}, time.Time{}
		case row := <-evidence:
			p := row.proof
			now := time.Now()
			if p.State != "exhausted" || p.ValidUntil == nil || !p.ValidUntil.After(now) {
				return claudeResetRoute{}, claudeResetProof{}, time.Time{}
			}
			if deadline.IsZero() || p.ValidUntil.Before(deadline) {
				deadline = *p.ValidUntil
			}
			if len(p.Offer) != sha256.Size*2 || p.CreditExpiry == nil || !p.CreditExpiry.After(now) {
				continue
			}
			if chosen.channel == nil || p.CreditExpiry.Before(*offer.CreditExpiry) ||
				p.CreditExpiry.Equal(*offer.CreditExpiry) && row.route.channel.Id < chosen.channel.Id {
				chosen, offer = row.route, p
			}
		}
	}
	return chosen, offer, deadline
}

func (p *RetryParam) PrepareClaudeResetReplay(channelID int) {
	p.ExhaustedChannels = slices.DeleteFunc(p.ExhaustedChannels, func(id int) bool { return id == channelID })
	p.SetRetry(common.RetryTimes)
}
