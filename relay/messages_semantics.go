// sudoapi: Require a compatible channel for explicit caller Messages semantics.

package relay

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"strings"
)

// A generic relay can drop headers or deserialize away an unknown body option.
// Refuse that route before conversion, network I/O, and retry on another channel.
func checkMessagesSemanticsRoute(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) error {
	const modeHeader = "X-SudoAPI-Messages-Semantics"
	const hashHeader = "X-SudoAPI-Messages-SHA256"
	modes, hashes := c.Request.Header.Values(modeHeader), c.Request.Header.Values(hashHeader)
	fromBody := len(request.MessagesSemantics) > 0
	if !fromBody && len(modes) == 0 && len(hashes) == 0 {
		return nil
	}
	conflict := errors.New("selected channel cannot preserve the requested Messages semantics")
	if len(modes) > 1 || len(hashes) > 1 || info.ApiType != constant.APITypeAnthropic ||
		(!info.ChannelSetting.PassThroughBodyEnabled && !model_setting.GetGlobalSettings().PassThroughRequestEnabled) {
		return conflict
	}
	// Only an explicit channel setting declares support. A general wildcard
	// could otherwise forward this preference to an unaware upstream.
	enabled := false
	for k := range relaycommon.GetEffectiveHeaderOverride(info) {
		if strings.EqualFold(strings.TrimSpace(k), modeHeader) {
			enabled = true
			break
		}
	}
	if !enabled {
		return conflict
	}
	h, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return conflict
	}
	wire := h[strings.ToLower(modeHeader)]
	if len(hashes) == 1 && (hashes[0] == "" || h[strings.ToLower(hashHeader)] != hashes[0]) {
		return conflict
	}
	if len(modes) == 1 && modes[0] != wire {
		return conflict
	}
	choice := wire
	if fromBody {
		if wire != "caller" || common.Unmarshal(request.MessagesSemantics, &choice) != nil {
			return conflict
		}
	}
	switch choice {
	case "exact", "sdk-managed", "sdk-fork":
		return nil
	}
	return conflict
}
