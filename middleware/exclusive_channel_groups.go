// sudoapi: Enforce explicit token groups for private channels.

package middleware

import (
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// Explicit token membership also applies to administrator channel overrides.
// A private channel cannot become an Auto fallback or an administrator API
// key's diagnostic target just because the caller knows its channel ID.
func allowsExclusiveChannelGroup(c *gin.Context, channel *model.Channel) bool {
	if channel == nil || common.GetContextKeyInt(c, constant.ContextKeyTokenId) == 0 {
		return true
	}
	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	for _, restricted := range strings.Split(os.Getenv("REQUIRE_EXPLICIT_TOKEN_GROUPS"), ",") {
		restricted = strings.TrimSpace(restricted)
		if restricted == "" {
			continue
		}
		for _, group := range strings.Split(channel.Group, ",") {
			if strings.TrimSpace(group) == restricted && tokenGroup != restricted {
				return false
			}
		}
	}
	return true
}
