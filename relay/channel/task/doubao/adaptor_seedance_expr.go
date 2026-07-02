package doubao

import (
	"net/http"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/hot"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
)

const estimatedCompletionTokens = 50_0000

func getSeedanceTieredBillingSnapshot(c *gin.Context, info *relaycommon.RelayInfo, req requestPayload) (*billingexpr.BillingSnapshot, *dto.TaskError) {
	exprStr, ok := billing_setting.GetBillingExpr(info.OriginModelName)
	if !ok {
		return nil, service.TaskErrorWrapperLocal(errors.Errorf("model %s tiered price not found", info.OriginModelName), "invalid_price", http.StatusPreconditionFailed)
	}
	trace, err := runExpr(exprStr, req, "", estimatedCompletionTokens)
	if err != nil {
		return nil, service.TaskErrorWrapperLocal(errors.Wrapf(err, "model %s tiered expr run failed", info.OriginModelName), "invalid_price", http.StatusPreconditionFailed)
	}
	if trace.MatchedTier == "" || trace.Cost == 0 {
		return nil, service.TaskErrorWrapperLocal(errors.New("invalid request"), "invalid_request", http.StatusBadRequest)
	}
	groupRatio := helper.HandleGroupRatio(c, info).GroupRatio
	quotaBeforeGroup := trace.Cost / 100_0000 * common.QuotaPerUnit
	quotaAfterGroup, err := billingexpr.QuotaRoundStrict(quotaBeforeGroup * groupRatio)
	if err != nil {
		return nil, service.TaskErrorWrapperLocal(err, "invalid_price", http.StatusBadRequest)
	}
	return &billingexpr.BillingSnapshot{
		BillingMode:               "tiered_expr",
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  billingexpr.ExprHashString(exprStr),
		GroupRatio:                groupRatio,
		EstimatedPromptTokens:     0,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  quotaAfterGroup,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.QuotaPerUnit,
		ExprVersion:               billingexpr.DefaultExprVersion,
	}, nil
}

var exprCache = hot.NewHotCache[string, *vm.Program](hot.LRU, 16).
	WithLoaders(func(keys []string) (map[string]*vm.Program, error) {
		rst := make(map[string]*vm.Program, len(keys))
		for _, key := range keys {
			program, err := expr.Compile(key, expr.Env(map[string]any{
				"req":          requestPayload{},
				"matched_tier": "",
				"tier":         func(name string, cost float64) float64 { return 0 },
				"c":            0,
			}))
			if err != nil {
				return nil, err
			}
			rst[key] = program
		}
		return rst, nil
	}).
	Build()

func runExpr(exprStr string, req requestPayload, matchedTier string, completionTokens int) (*billingexpr.TraceResult, error) {
	program, found, err := exprCache.Get(exprStr)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("tiered expr not found")
	}

	var trace billingexpr.TraceResult
	_, err = expr.Run(program, map[string]any{
		"req":          req,
		"matched_tier": matchedTier,
		"tier": func(matchedTier string, cost float64) float64 {
			trace.MatchedTier = matchedTier
			trace.Cost = cost
			return cost
		},
		"c": completionTokens,
	})
	if err != nil {
		return nil, errors.Wrap(err, "invalid tiered expr")
	}

	return &trace, nil
}
