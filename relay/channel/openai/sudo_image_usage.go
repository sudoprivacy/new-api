// sudoapi: Protocol-specific OpenAI usage serialization.

package openai

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// decodeOpenAIImageResponse decodes only usage, error, and event type, keeping
// large image data out of Go strings and preserving the upstream response body.
func decodeOpenAIImageResponse(data []byte) (*dto.SimpleResponse, string, error) {
	var wire struct {
		Usage json.RawMessage `json:"usage"`
		Error any             `json:"error"`
		Type  string          `json:"type"`
	}
	if err := common.Unmarshal(data, &wire); err != nil {
		return nil, "", err
	}
	response := &dto.SimpleResponse{Error: wire.Error}
	if len(wire.Usage) > 0 {
		usage, err := dto.DecodeOpenAIInputUsage(wire.Usage)
		if err != nil {
			return nil, "", err
		}
		if usage != nil {
			response.Usage = *usage
		}
	}
	return response, wire.Type, nil
}
