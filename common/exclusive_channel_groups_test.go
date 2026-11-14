// sudoapi: Enforce explicit token groups for private channels.

package common

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExclusiveChannelPublicationGroups(t *testing.T) {
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", " owner-private, partner-private,owner-private ")
	for _, tc := range []struct {
		name, groups string
		rejected     bool
	}{
		{"shared customer groups", "customer,another-customer", false},
		{"one explicit private group", " owner-private, owner-private, ", false},
		{"public and private", "customer,owner-private", true},
		{"different private scopes", "owner-private,partner-private", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateExclusiveChannelGroups(tc.groups)
			if tc.rejected {
				require.ErrorIs(t, err, ErrMixedExclusiveChannelGroups)
			} else {
				require.NoError(t, err)
			}
		})
	}
	t.Setenv("REQUIRE_EXPLICIT_TOKEN_GROUPS", "")
	require.NoError(t, ValidateExclusiveChannelGroups("customer,owner-private"))
}
