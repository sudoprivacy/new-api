// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type Setting struct {
	Enabled        bool        `json:"enabled"`
	MaxTextBytes   int         `json:"max_text_bytes"`
	MaxPayloadSize int         `json:"max_payload_size"`
	File           fileSetting `json:"file"`
}

func (settings Setting) captureEnabled() bool {
	return settings.Enabled && settings.MaxTextBytes > 0 && settings.MaxPayloadSize > 0
}

func (settings Setting) fileCaptureEnabled() bool {
	return settings.captureEnabled() && settings.File.Enabled
}

type fileSetting struct {
	Enabled    bool   `json:"enabled"`
	Filename   string `json:"file_name"`
	MaxSize    int64  `json:"max_size"`
	MaxBackups int    `json:"max_backups"`
}

var setting = Setting{
	MaxTextBytes:   1 << 20,  // 1 MiB
	MaxPayloadSize: 64 << 20, // 64 MiB
	File: fileSetting{
		Enabled:    false,
		Filename:   "/app/logs/text/text",
		MaxSize:    100,
		MaxBackups: 10,
	},
}

func init() {
	config.GlobalConfig.Register("model_capture_setting", &setting)
}

func captureSettings() Setting {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return setting
}
