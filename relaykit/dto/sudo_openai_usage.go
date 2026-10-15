// sudoapi: Protocol-specific OpenAI usage serialization.

package dto

import (
	"encoding/json"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

// DecodeOpenAIInputUsage reads the Responses and Images usage shape without
// adding its fields to Chat Completions output or changing embedded Usage DTOs.
func DecodeOpenAIInputUsage(data []byte) (*Usage, error) {
	type usageAlias Usage
	var wire *struct {
		usageAlias
		InputTokens         int                 `json:"input_tokens"`
		OutputTokens        int                 `json:"output_tokens"`
		InputTokensDetails  *InputTokenDetails  `json:"input_tokens_details"`
		OutputTokensDetails *OutputTokenDetails `json:"output_tokens_details"`
	}
	if err := kitutil.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	if wire == nil {
		return nil, nil
	}
	usage := Usage(wire.usageAlias)
	usage.InputTokens = wire.InputTokens
	usage.OutputTokens = wire.OutputTokens
	usage.InputTokensDetails = wire.InputTokensDetails
	if wire.OutputTokensDetails != nil {
		usage.CompletionTokenDetails = *wire.OutputTokensDetails
	}
	return &usage, nil
}

func (r *OpenAIResponsesResponse) UnmarshalJSON(data []byte) error {
	type responseAlias OpenAIResponsesResponse
	var wire struct {
		responseAlias
		Usage json.RawMessage `json:"usage"`
	}
	if err := kitutil.Unmarshal(data, &wire); err != nil {
		return err
	}
	if len(wire.Usage) > 0 {
		usage, err := DecodeOpenAIInputUsage(wire.Usage)
		if err != nil {
			return err
		}
		wire.responseAlias.Usage = usage
	}
	*r = OpenAIResponsesResponse(wire.responseAlias)
	return nil
}

func (r OpenAIResponsesResponse) MarshalJSON() ([]byte, error) {
	type responseAlias OpenAIResponsesResponse
	type responseUsage struct {
		InputTokens         int                `json:"input_tokens"`
		OutputTokens        int                `json:"output_tokens"`
		TotalTokens         int                `json:"total_tokens"`
		InputTokensDetails  *InputTokenDetails `json:"input_tokens_details"`
		OutputTokensDetails OutputTokenDetails `json:"output_tokens_details"`
	}
	var usage *responseUsage
	if r.Usage != nil {
		usage = &responseUsage{
			InputTokens:         r.Usage.InputTokens,
			OutputTokens:        r.Usage.OutputTokens,
			TotalTokens:         r.Usage.TotalTokens,
			InputTokensDetails:  r.Usage.InputTokensDetails,
			OutputTokensDetails: r.Usage.CompletionTokenDetails,
		}
		if usage.InputTokens == 0 {
			usage.InputTokens = r.Usage.PromptTokens
		}
		if usage.OutputTokens == 0 {
			usage.OutputTokens = r.Usage.CompletionTokens
		}
		if usage.InputTokensDetails == nil && r.Usage.PromptTokensDetails != (InputTokenDetails{}) {
			details := r.Usage.PromptTokensDetails
			usage.InputTokensDetails = &details
		}
	}
	return kitutil.Marshal(struct {
		responseAlias
		Usage *responseUsage `json:"usage"`
	}{responseAlias(r), usage})
}
