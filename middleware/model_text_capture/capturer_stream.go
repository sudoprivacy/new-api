// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"bytes"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func newStreamCapturer(format types.RelayFormat, maxTextBytes, maxPayloadSize int) *streamCapturer {
	return &streamCapturer{
		format: format,
		output: NewLimitedBuilder(maxTextBytes),
		framer: sseFramer{maxPayloadSize: maxPayloadSize},
	}
}

type streamCapturer struct {
	format            types.RelayFormat
	output            LimitedBuilder
	responsesFallback string
	framer            sseFramer
}

func (capturer *streamCapturer) captureString(data string) {
	const chunkBytes = 32 << 10
	for len(data) > 0 && !capturer.output.Full() {
		size := min(len(data), chunkBytes)
		capturer.capture([]byte(data[:size]))
		data = data[size:]
	}
}

func (capturer *streamCapturer) capture(data []byte) {
	for len(data) > 0 && !capturer.output.Full() {
		event, rest := capturer.framer.nextEvent(data)
		if len(event) > 0 {
			capturer.consumeEvent(event)
		}
		data = rest
	}
}

func (capturer *streamCapturer) consumeEvent(data []byte) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
		return
	}
	switch capturer.format {
	case types.RelayFormatOpenAI:
		var event dto.ChatCompletionsStreamResponse
		if common.Unmarshal(data, &event) != nil {
			return
		}
		for _, choice := range event.Choices {
			capturer.output.WriteString(choice.Delta.GetContentString())
		}
	case types.RelayFormatOpenAIResponses:
		var event struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Response *struct {
				Output []dto.ResponsesOutput `json:"output"`
			} `json:"response"`
		}
		if common.Unmarshal(data, &event) != nil {
			return
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				capturer.responsesFallback = ""
				capturer.output.WriteString(event.Delta)
			}
		case "response.completed", "response.done", "response.incomplete", "response.failed", "response.cancelled", "response.canceled":
			if capturer.output.String() == "" && capturer.responsesFallback == "" && event.Response != nil {
				fallback := NewLimitedBuilder(capturer.output.remaining)
				appendResponsesText(&fallback, event.Response.Output)
				capturer.responsesFallback = fallback.String()
			}
		}
	case types.RelayFormatClaude:
		var event dto.ClaudeResponse
		if common.Unmarshal(data, &event) != nil {
			return
		}
		switch event.Type {
		case "content_block_start":
			if block := event.ContentBlock; block != nil && block.Type == "text" && block.Text != nil {
				capturer.output.WriteString(*block.Text)
			}
		case "content_block_delta":
			if delta := event.Delta; delta != nil && delta.Type == "text_delta" && delta.Text != nil {
				capturer.output.WriteString(*delta.Text)
			}
		}
	case types.RelayFormatGemini:
		var event dto.GeminiChatResponse
		if common.Unmarshal(data, &event) != nil {
			return
		}
		for _, candidate := range event.Candidates {
			for _, part := range candidate.Content.Parts {
				if !part.Thought {
					capturer.output.WriteString(part.Text)
				}
			}
		}
	}
}

func (capturer *streamCapturer) result() (string, error) {
	if capturer.responsesFallback != "" {
		return capturer.responsesFallback, nil
	}
	return capturer.output.String(), nil
}
