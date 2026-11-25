// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/relaykit/types"
)

func TestStreamCapturerTextOrder(t *testing.T) {
	tests := []struct {
		name   string
		format types.RelayFormat
		events []string
		want   string
	}{
		{
			name:   "chat choices in wire order",
			format: types.RelayFormatOpenAI,
			events: []string{
				`{"choices":[{"index":2,"delta":{"content":"A","reasoning_content":"ignored"}},{"index":0,"delta":{"content":"B"}}]}`,
				`{"choices":[{"index":2,"delta":{"tool_calls":[{"function":{"arguments":"ignored"}}]}}]}`,
				`{"choices":[{"index":2,"delta":{"content":"C"}}]}`,
				`{"choices":[],"usage":{"completion_tokens":10}}`,
				`[DONE]`,
			},
			want: "ABC",
		},
		{
			name:   "responses text deltas override final text",
			format: types.RelayFormatOpenAIResponses,
			events: []string{
				`{"type":"response.output_text.delta","output_index":2,"content_index":1,"delta":"A"}`,
				`{"type":"response.function_call_arguments.delta","delta":"ignored"}`,
				`{"type":"response.reasoning_summary_text.delta","delta":"ignored"}`,
				`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"B"}`,
				`{"type":"response.output_text.done","text":"ignored"}`,
				`{"type":"response.completed","response":{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"BA"}]}]}}`,
			},
			want: "AB",
		},
		{
			name:   "responses terminal fallback only once and in output order",
			format: types.RelayFormatOpenAIResponses,
			events: []string{
				`{"type":"response.output_text.delta","delta":""}`,
				`{"type":"response.incomplete","response":{"output":[{"type":"function_call","arguments":"ignored"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"A"},{"type":"refusal","refusal":"ignored"},{"type":"output_text","text":"B"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"C"}]}]}}`,
				`{"type":"response.done","response":{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ABC"}]}]}}`,
			},
			want: "ABC",
		},
		{
			name:   "claude block events in wire order",
			format: types.RelayFormatClaude,
			events: []string{
				`{"type":"content_block_start","index":2,"content_block":{"type":"text","text":"A"}}`,
				`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"B"}}`,
				`{"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"ignored"}}`,
				`{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"ignored"}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":"C"}}`,
			},
			want: "ABC",
		},
		{
			name:   "gemini parts in wire order regardless of candidate finish",
			format: types.RelayFormatGemini,
			events: []string{
				`{"candidates":[{"index":2,"finishReason":"STOP","content":{"parts":[{"text":"A"},{"text":"ignored","thought":true},{"text":"B"}]}}]}`,
				`{"candidates":[{"index":0,"content":{"parts":[{"functionCall":{"name":"ignored","args":{"value":"ignored"}}},{"text":"C"}]}},{"index":1,"content":{"parts":[{"text":"D"}]}}]}`,
			},
			want: "ABCD",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capturer := newStreamCapturer(test.format, 64, 1024)
			capturer.capture([]byte("data: " + strings.Join(test.events, "\n\ndata: ") + "\n\n"))
			text, err := capturer.result()
			require.NoError(t, err)
			assert.Equal(t, test.want, text)
		})
	}
}

func TestStreamCapturerRequiresCompleteSSEEvents(t *testing.T) {
	capturer := newStreamCapturer(types.RelayFormatOpenAI, 64, 1024)
	for _, chunk := range []string{
		"\xEF\xBB", "\xBF: heartbeat\r", "\n\r\n",
		"data: invalid JSON\n\n",
		"event: message\rdata: {\r\ndata: \"choices\":[{\"delta\":{\"content\":\"A", "\"}}]}\r", "\n",
	} {
		capturer.capture([]byte(chunk))
	}
	text, err := capturer.result()
	require.NoError(t, err)
	assert.Empty(t, text)

	capturer.capture([]byte("\rdata: {\"choices\":[{\"delta\":{\"content\":\"not a complete event\"}}]}\n"))
	text, err = capturer.result()
	require.NoError(t, err)
	assert.Equal(t, "A", text)
}

func TestStreamCapturerRespectsByteLimit(t *testing.T) {
	capturer := newStreamCapturer(types.RelayFormatOpenAIResponses, 4, 1024)
	capturer.capture([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"x\"}\n\n"))
	capturer.capture([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"中😀\"}\n\n"))
	capturer.capture([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"y\"}\n\n"))
	text, err := capturer.result()
	require.NoError(t, err)
	assert.Equal(t, "x中", text)

	capturer = newStreamCapturer(types.RelayFormatOpenAI, 4, 1024)
	capturer.capture([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x😀\"}},{\"delta\":{\"content\":\"y\"}}]}\n\n"))
	text, err = capturer.result()
	require.NoError(t, err)
	assert.Equal(t, "x", text)
}

func TestStreamCapturerSkipsOversizedSSEEvent(t *testing.T) {
	capturer := newStreamCapturer(types.RelayFormatOpenAI, 64, 1024)

	capturer.capture([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"A\"}}]}\n\n"))
	capturer.capture([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ignored\",\"tool_calls\":[{\"function\":{\"arguments\":\""))
	capturer.captureString(strings.Repeat("x", capturer.framer.maxPayloadSize))
	capturer.capture([]byte("\"}}]}}]}\r"))
	capturer.capture([]byte("\n\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"B\"}}]}\n\n"))

	text, err := capturer.result()
	require.NoError(t, err)
	assert.Equal(t, "AB", text)
}

func TestStreamCapturerDiscardsAllDataLinesOfOversizedEvent(t *testing.T) {
	capturer := newStreamCapturer(types.RelayFormatOpenAI, 64, 1024)
	padding := "data: " + strings.Repeat(" ", capturer.framer.maxPayloadSize/2) + "\n"
	capturer.captureString(padding)
	capturer.captureString(padding)
	capturer.captureString("data: {\"choices\":[{\"delta\":{\"content\":\"ignored\"}}]}\n\n")
	capturer.captureString("data: {\"choices\":[{\"delta\":{\"content\":\"kept\"}}]}\n\n")

	text, err := capturer.result()
	require.NoError(t, err)
	assert.Equal(t, "kept", text)
}
