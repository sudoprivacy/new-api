// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func modelTextInput(body common.BodyStorage, format types.RelayFormat, maxTextBytes int) (string, error) {
	reader, err := body.NewReader()
	if err != nil {
		return "", err
	}
	defer func() { _ = reader.Close() }()

	builder := NewLimitedBuilder(maxTextBytes)
	switch format {
	case types.RelayFormatGemini:
		var request dto.GeminiChatRequest
		if err := common.DecodeJson(reader, &request); err != nil {
			return "", err
		}
		if request.SystemInstructions != nil {
			appendInput(&builder, "system", geminiText(request.SystemInstructions.Parts))
		}
		for _, content := range request.Contents {
			if builder.Full() {
				break
			}
			appendInput(&builder, content.Role, geminiText(content.Parts))
		}
	case types.RelayFormatOpenAIResponses:
		var request struct {
			Instructions any `json:"instructions"`
			Input        any `json:"input"`
		}
		if err := common.DecodeJson(reader, &request); err != nil {
			return "", err
		}
		appendResponsesInput(&builder, request.Instructions, request.Input)
	default:
		var request struct {
			Messages []dto.Message `json:"messages"`
			System   any           `json:"system"`
			Prefix   any           `json:"prefix"`
			Suffix   any           `json:"suffix"`
		}
		if err := common.DecodeJson(reader, &request); err != nil {
			return "", err
		}
		if format == types.RelayFormatClaude {
			appendInput(&builder, "system", contentText(request.System, "text"))
		}
		for _, message := range request.Messages {
			if builder.Full() {
				break
			}
			appendInput(&builder, message.Role, contentText(message.Content, "text"))
		}
		if format == types.RelayFormatOpenAI {
			prefix, _ := request.Prefix.(string)
			suffix, _ := request.Suffix.(string)
			appendLabeledText(&builder, "prefix", prefix)
			appendLabeledText(&builder, "suffix", suffix)
		}
	}
	return builder.String(), nil
}

func appendResponsesInput(builder *LimitedBuilder, instructions, input any) {
	if builder.Full() {
		return
	}
	appendInput(builder, "system", contentText(instructions, "input_text"))
	switch value := input.(type) {
	case string:
		appendInput(builder, "user", value)
	case []any:
		for _, item := range value {
			if builder.Full() {
				break
			}
			message, ok := item.(map[string]any)
			if !ok || (message["type"] != nil && message["type"] != "message") {
				continue
			}
			role, _ := message["role"].(string)
			blockType := "input_text"
			if role == "assistant" {
				blockType = "output_text"
			}
			appendInput(builder, role, contentText(message["content"], blockType))
		}
	}
}

func appendInput(builder *LimitedBuilder, role, text string) {
	switch role {
	case "system", "developer", "user", "assistant", "model":
	case "":
		role = "user"
	default:
		return
	}
	appendLabeledText(builder, role, text)
}

func appendLabeledText(builder *LimitedBuilder, label, text string) {
	if text == "" {
		return
	}
	builder.WriteString(label)
	builder.WriteString(": ")
	builder.WriteString(text)
	builder.WriteString("\n")
}
