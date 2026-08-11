// sudoapi: API for sudowork

package v1

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

func GetSpecificPricing(ctx *gin.Context) {
	common.ApiSuccess(ctx, sudoworkSettings.SpecificPricing)
}

func GetSpecificImagePricing(ctx *gin.Context) {
	common.ApiSuccess(ctx, sudoworkSettings.SpecificImagePricing)
}

var sudoworkSettings struct {
	SpecificPricing      json.RawMessage `json:"specific_pricing"`
	SpecificImagePricing json.RawMessage `json:"specific_image_pricing"`
}

func init() {
	config.GlobalConfig.Register("sudowork_setting", &sudoworkSettings)
}
