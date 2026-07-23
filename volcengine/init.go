// sudoapi: Volcengine ark asset.

package volcengine

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
)

func Init() error {
	return model.DB.AutoMigrate(&ArkAssetGroup{}, &ArkAsset{}, &ArkVisualValidateSession{})
}

type ArkSetting struct {
	Enabled     bool   `json:"enabled"`
	BaseURL     string `json:"base_url"`
	AK          string `json:"ak"`
	SK          string `json:"sk"`
	ProxyURL    string `json:"proxy_url"`
	CallbackURL string `json:"callback_url"`
}

var arkSetting = ArkSetting{}

func init() {
	config.GlobalConfig.Register("ark_setting", &arkSetting)
}
