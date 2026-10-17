// sudoapi: Enforce explicit token groups for private channels.

package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExclusiveChannelGroups(t *testing.T) {
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", " fujitoken, partner-private ")
	for _, tc := range []struct {
		name, tokenGroup, channelGroups string
		role, channelID                 int
		allowed                         bool
	}{
		{"admin CI forced private channel", "auto", "fujitoken", 100, 42, false},
		{"personal admin forced private channel", "auto", "fujitoken", 10, 42, false},
		{"inherited private user group is insufficient", "", "fujitoken", 100, 42, false},
		{"explicit private token", "fujitoken", "fujitoken", 1, 42, true},
		{"CI paid native channel", "auto", "Bedrock", 100, 54, true},
		{"CI paid OpenAI channel", "auto", "OpenAI,default", 100, 38, true},
		{"private channel also published in public group", "auto", "default, fujitoken", 100, 42, false},
		{"another configured private group", "auto", "partner-private", 100, 98, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(c, constant.ContextKeyTokenId, 411)
			common.SetContextKey(c, constant.ContextKeyTokenGroup, tc.tokenGroup)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "fujitoken")
			c.Set("role", tc.role)
			c.Set("specific_channel_id", tc.channelID)
			require.Equal(t, tc.allowed, allowsExclusiveChannelGroup(c, &model.Channel{Id: tc.channelID, Group: tc.channelGroups}))
		})
	}
}

func TestExclusiveChannelGroupsDisabled(t *testing.T) {
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyTokenId, 411)
	require.True(t, allowsExclusiveChannelGroup(c, &model.Channel{Group: "private"}))
}

func TestExclusiveChannelRejectsRetryBeforeReadingUpstreamKey(t *testing.T) {
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "fujitoken")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyTokenId, 411)
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
	err := SetupContextForSelectedChannel(c, &model.Channel{Id: 42, Group: "fujitoken", Key: "test-upstream-key"}, "claude-sonnet-4-6")
	require.NotNil(t, err)
	require.Equal(t, 403, err.StatusCode)
	_, readKey := c.Get(string(constant.ContextKeyChannelKey))
	require.False(t, readKey)
}

func TestExclusiveChannelDoesNotReplaceConsoleAuthorization(t *testing.T) {
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "fujitoken")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, allowsExclusiveChannelGroup(c, &model.Channel{Group: "fujitoken"}))
}
