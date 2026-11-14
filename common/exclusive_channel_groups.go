// sudoapi: Enforce explicit token groups for private channels.

package common

import (
	"errors"
	"os"
	"strings"
)

var ErrMixedExclusiveChannelGroups = errors.New("a private channel group cannot be combined with other groups; use separate channels")

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
