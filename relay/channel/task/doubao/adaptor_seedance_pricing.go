package doubao

import (
	"fmt"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

func getSeedancePrice(model string, req *requestPayload) (float64, error) {
	price, ok := seedancePrices[model]
	if !ok {
		return 0, errors.Errorf("model %s price not found", model)
	}
	key, err := expr.Run(price.program, req)
	if err != nil {
		return 0, err
	}
	k, ok := key.(string)
	if !ok {
		return 0, errors.Errorf("invalid expr result: %v, model: %s", k, model)
	}
	p, ok := price.prices[k]
	if !ok {
		return 0, errors.Errorf("price %s not found, model: %s", k, model)
	}
	return p, nil
}

type seedancePrice struct {
	program *vm.Program
	prices  map[string]float64
}

var seedancePrices map[string]seedancePrice

func init() {
	type price struct {
		model  string
		expr   string
		prices map[string]float64
	}
	unitPrice := 7.3
	// 官方价格: https://console.volcengine.com/ark/region:cn-beijing/model/detail?Id=doubao-seedance-2-0
	// 官方价格计算器: https://bytedance.larkoffice.com/share/base/form/shrcnP1Bl0mqCP9OHCbjpe1oBkf
	prices := []price{
		{
			model: "doubao-seedance-2-0",
			expr:  `Resolution + " " + (any(Content, .Type == "video_url") ? "with video" : "without video")`,
			prices: map[string]float64{
				"480p with video":    28 / unitPrice,
				"480p without video": 46 / unitPrice,
				"720p with video":    28 / unitPrice,
				"720p without video": 46 / unitPrice,

				"1080p with video":    31 / unitPrice,
				"1080p without video": 51 / unitPrice,

				"4k with video":    16 / unitPrice,
				"4k without video": 26 / unitPrice,
			},
		}, {
			model: "doubao-seedance-2-0-fast",
			expr:  `(any(Content, .Type == "video_url") ? "with video" : "without video")`,
			prices: map[string]float64{
				"with video":    22 / unitPrice,
				"without video": 37 / unitPrice,
			},
		}, {
			model: "doubao-seedance-2-0-mini",
			expr:  `(any(Content, .Type == "video_url") ? "with video" : "without video")`,
			prices: map[string]float64{
				"with video":    14 / unitPrice,
				"without video": 23 / unitPrice,
			},
		},
	}
	seedancePrices = lo.SliceToMap(prices, func(price price) (string, seedancePrice) {
		program, err := expr.Compile(price.expr, expr.Env(requestPayload{}))
		if err != nil {
			panic(fmt.Sprintf("compile expr failed, model: %s, error: %v", price.model, err))
		}
		return price.model, seedancePrice{program: program, prices: price.prices}
	})
}
