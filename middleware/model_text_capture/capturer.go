// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"bytes"
	"strings"

	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

type textCapturer interface {
	capture(data []byte)
	captureString(data string)
	result() (string, error)
}

func newNonStreamCapturer(format types.RelayFormat, maxTextBytes, maxPayloadSize int) *nonStreamCapturer {
	return &nonStreamCapturer{
		maxTextBytes:   maxTextBytes,
		maxPayloadSize: maxPayloadSize,
		buffer:         bytes.NewBuffer(nil),
		format:         format,
	}
}

type nonStreamCapturer struct {
	maxTextBytes   int
	maxPayloadSize int

	buffer *bytes.Buffer
	format types.RelayFormat
}

func (capturer *nonStreamCapturer) capture(data []byte) {
	if capturer.canBuffer(len(data)) {
		capturer.buffer.Write(data)
	}
}

func (capturer *nonStreamCapturer) captureString(data string) {
	if capturer.canBuffer(len(data)) {
		capturer.buffer.WriteString(data)
	}
}

func (capturer *nonStreamCapturer) canBuffer(size int) bool {
	if capturer.buffer == nil {
		return false
	}
	if capturer.buffer.Len()+size > capturer.maxPayloadSize {
		capturer.buffer = nil
		return false
	}
	return true
}

func (capturer *nonStreamCapturer) result() (string, error) {
	if capturer.buffer == nil {
		return "", nil
	}
	builder := NewLimitedBuilder(capturer.maxTextBytes)
	data := capturer.buffer.Bytes()
	switch capturer.format {
	case types.RelayFormatOpenAI:
		var response dto.OpenAITextResponse
		if err := common.Unmarshal(data, &response); err != nil {
			return "", err
		}
		for _, choice := range response.Choices {
			if builder.Full() {
				break
			}
			builder.WriteString(contentText(choice.Message.Content, "text"))
		}
	case types.RelayFormatOpenAIResponses:
		var response dto.OpenAIResponsesResponse
		if err := common.Unmarshal(data, &response); err != nil {
			return "", err
		}
		appendResponsesText(&builder, response.Output)
	case types.RelayFormatClaude:
		var response dto.ClaudeResponse
		if err := common.Unmarshal(data, &response); err != nil {
			return "", err
		}
		for _, block := range response.Content {
			if builder.Full() {
				break
			}
			if block.Type == "text" && block.Text != nil {
				builder.WriteString(*block.Text)
			}
		}
	case types.RelayFormatGemini:
		var response dto.GeminiChatResponse
		if err := common.Unmarshal(data, &response); err != nil {
			return "", err
		}
		for _, candidate := range response.Candidates {
			if builder.Full() {
				break
			}
			builder.WriteString(geminiText(candidate.Content.Parts))
		}
	default:
		return "", errors.Errorf("invalid relay format %v", capturer.format)
	}
	return builder.String(), nil
}

func appendResponsesText(builder *LimitedBuilder, output []dto.ResponsesOutput) {
	for _, item := range output {
		if item.Type != "message" || (item.Role != "assistant" && item.Role != "") {
			continue
		}
		for _, content := range item.Content {
			if builder.Full() {
				return
			}
			if content.Type == "output_text" {
				builder.WriteString(content.Text)
			}
		}
	}
}

func contentText(content any, blockType string) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var text strings.Builder
		for _, item := range value {
			block, ok := item.(map[string]any)
			if ok && block["type"] == blockType {
				part, _ := block["text"].(string)
				text.WriteString(part)
			}
		}
		return text.String()
	default:
		return ""
	}
}

func geminiText(parts []dto.GeminiPart) string {
	var text strings.Builder
	for _, part := range parts {
		if !part.Thought {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}
