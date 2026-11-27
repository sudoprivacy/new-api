// sudoapi: Enforce explicit token groups for private channels.

package common

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

var ErrMixedExclusiveChannelGroups = errors.New("a private channel group cannot be combined with other groups; use separate channels")

// A dedicated public token and channel are reserved for one another across
// initial selection, affinity, forced channel selection and retries.
type DedicatedTokenChannel struct{ TokenID, ChannelID int }

var SDKTestRoute DedicatedTokenChannel

func ParseDedicatedTokenChannel(value string) (DedicatedTokenChannel, error) {
	if value == "" {
		return DedicatedTokenChannel{}, nil
	}
	left, right, ok := strings.Cut(value, ":")
	token, tokenErr := strconv.Atoi(left)
	channel, channelErr := strconv.Atoi(right)
	if !ok || tokenErr != nil || channelErr != nil || token < 1 || channel < 1 {
		return DedicatedTokenChannel{}, errors.New("SDK_TEST_TOKEN_CHANNEL must contain positive token_id:channel_id")
	}
	return DedicatedTokenChannel{token, channel}, nil
}

func (pair DedicatedTokenChannel) ReservesToken(tokenID int) bool {
	return pair.TokenID > 0 && pair.TokenID == tokenID
}

func (pair DedicatedTokenChannel) Allows(tokenID, channelID int) bool {
	return pair.TokenID == 0 || (pair.TokenID == tokenID) == (pair.ChannelID == channelID)
}

// ExplicitTokenGroups is shared by serving authorization and publication checks.
func ExplicitTokenGroups() map[string]struct{} {
	groups := make(map[string]struct{})
	for _, value := range strings.Split(os.Getenv("REQUIRE_EXPLICIT_TOKEN_GROUPS"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			groups[value] = struct{}{}
		}
	}
	return groups
}

// ValidateExclusiveChannelGroups prevents one entry from having contradictory
// public and private serving policies. It neither changes access nor repairs data.
func ValidateExclusiveChannelGroups(value string) error {
	groups := make(map[string]struct{})
	for _, group := range strings.Split(value, ",") {
		if group = strings.TrimSpace(group); group != "" {
			groups[group] = struct{}{}
		}
	}
	if len(groups) < 2 {
		return nil
	}
	for group := range ExplicitTokenGroups() {
		if _, present := groups[group]; present {
			return ErrMixedExclusiveChannelGroups
		}
	}
	return nil
}
