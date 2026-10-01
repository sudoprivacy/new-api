// sudoapi: Discover models, capabilities, and reference prices from trusted upstream catalogs.
package upstream_catalog

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/require"
)

func TestCatalogPricingRequiresCompletePricesAndPreservesTiers(t *testing.T) {
	raw := `{"model":{"channel_id":42,"reference_pricing":{"currency":"USD","unit":"per_million_tokens","input":2,"output":10,"cache_read":0,"cache_write_5m":2.5,"cache_write_1h":4,"long_context_threshold":200000,"long_context_input_multiplier":2,"long_context_output_multiplier":1.5}}}`
	entries, err := Decode(raw)
	require.NoError(t, err)
	p := entries["model"].Pricing
	expression, err := p.Expression()
	require.NoError(t, err)
	for _, tc := range []struct{ length, want float64 }{{200000, 470}, {200001, 890}} {
		cost, _, err := billingexpr.RunExprWithRequest(expression, billingexpr.TokenParams{
			P: 100, C: 10, CR: 1000, CC: 20, CC1h: 30, Len: tc.length,
		}, billingexpr.RequestInput{})
		require.NoError(t, err)
		require.InDelta(t, tc.want, cost, 1e-8)
	}
	p.CacheWrite1h = nil
	_, err = p.Expression()
	require.Error(t, err, "missing 1h cache price must not become a free cache write")
}
