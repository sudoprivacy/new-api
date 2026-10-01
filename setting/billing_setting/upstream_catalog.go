// sudoapi: Discover models, capabilities, and reference prices from trusted upstream catalogs.
package billing_setting

import (
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/upstream_catalog"
)

func upstreamBillingExpression(model string) (string, bool) {
	if _, exists := billingSetting.BillingMode[model]; exists {
		return "", false
	}
	if _, exists := ratio_setting.GetModelPrice(model, false); exists {
		return "", false
	}
	if ratio_setting.HasModelTokenRatio(model) {
		return "", false
	}
	return upstream_catalog.BillingExpression(model)
}
