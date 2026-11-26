// sudoapi: Coordinate Claude resets across native channels.

package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestCoordinatedClaudeResetUsesEarliestCreditOnlyWithCompleteEvidence(t *testing.T) {
	now := time.Now()
	fujiDeadline := now.Add(15 * time.Second)
	merlionDeadline := now.Add(30 * time.Second)
	fujiExpiry := now.Add(2 * time.Hour)
	merlionExpiry := now.Add(time.Hour)
	routes := []claudeResetRoute{{channel: &model.Channel{Id: 110}}, {channel: &model.Channel{Id: 115}}}
	query := func(route claudeResetRoute, _ context.Context, _ string, _ claudeResetRequest) claudeResetProof {
		if route.channel.Id == 110 {
			return claudeResetProof{State: "exhausted", ValidUntil: &fujiDeadline, CreditExpiry: &fujiExpiry, Offer: strings.Repeat("a", 64)}
		}
		return claudeResetProof{State: "exhausted", ValidUntil: &merlionDeadline, CreditExpiry: &merlionExpiry, Offer: strings.Repeat("b", 64)}
	}
	chosen, _, deadline := selectClaudeResetOffer(t.Context(), claudeResetRequest{}, routes, query)
	require.NotNil(t, chosen.channel)
	require.Equal(t, 115, chosen.channel.Id)
	require.Equal(t, fujiDeadline, deadline)
	for _, rejected := range []claudeResetProof{
		{State: "available"}, {State: "unknown"}, {},
		{State: "exhausted", ValidUntil: &now},
	} {
		chosen, _, _ := selectClaudeResetOffer(t.Context(), claudeResetRequest{}, routes,
			func(route claudeResetRoute, ctx context.Context, action string, input claudeResetRequest) claudeResetProof {
				if route.channel.Id == 110 {
					return rejected
				}
				return query(route, ctx, action, input)
			})
		require.Nil(t, chosen.channel)
	}
}
