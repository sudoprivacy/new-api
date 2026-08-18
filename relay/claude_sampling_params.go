// sudoapi: Remove unsupported sampling parameters for Claude Opus models.

package relay

import (
	"strings"

	"github.com/tidwall/sjson"
)

func removeUnsupportedClaudeSamplingParameters(model string, data []byte) ([]byte, error) {
	if !claudeModelRejectsSamplingParameters(model) {
		return data, nil
	}

	var err error
	for _, parameter := range []string{"temperature", "top_p", "top_k"} {
		data, err = sjson.DeleteBytes(data, parameter)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

func claudeModelRejectsSamplingParameters(model string) bool {
	return strings.HasPrefix(model, "claude-opus-4-7") ||
		strings.HasPrefix(model, "claude-opus-4-8")
}
