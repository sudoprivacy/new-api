// sudoapi: Enforce documented deployment tool capabilities before relay billing.
package helper

import (
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
)

func validateModelToolCapability(modelName string, request dto.Request) error {
	meta := model_setting.GetModelMetadata(modelName)
	if meta == nil || meta.ToolCallingSupported == nil || *meta.ToolCallingSupported {
		return nil
	}
	var lists []any
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		lists = []any{r.Tools, r.Functions}
	case *dto.ClaudeRequest:
		lists = []any{r.Tools}
	case *dto.OpenAIResponsesRequest:
		lists = []any{r.Tools}
	case *dto.OpenAIResponsesCompactionRequest:
		lists = []any{r.Tools}
	case *dto.GeminiChatRequest:
		lists = []any{r.Tools}
		for i := range r.Requests {
			if err := validateModelToolCapability(modelName, &r.Requests[i]); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	for _, list := range lists {
		raw, err := common.Marshal(list)
		if err != nil {
			return fmt.Errorf("invalid tool declarations")
		}
		var declarations []json.RawMessage
		if err := common.Unmarshal(raw, &declarations); err != nil {
			return fmt.Errorf("invalid tool declarations")
		}
		if len(declarations) > 0 {
			return fmt.Errorf("model %q does not support tool calling on this endpoint", modelName)
		}
	}
	return nil
}
