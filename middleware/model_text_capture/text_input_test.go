// sudoapi: Model request and response text capture.

package model_text_capture

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func TestModelTextPromptsPreservesRolesAndTextOrder(t *testing.T) {
	tests := []struct {
		name   string
		format types.RelayFormat
		body   string
		want   string
	}{
		{
			name:   "Chat FIM without messages",
			format: types.RelayFormatOpenAI,
			body:   `{"prefix":"func main() {","suffix":"}"}`,
			want:   "prefix: func main() {\nsuffix: }\n",
		},
		{
			name:   "Claude system and messages without tool results",
			format: types.RelayFormatClaude,
			body:   `{"system":[{"type":"text","text":"rules"}],"messages":[{"role":"user","content":[{"type":"tool_result","content":"ignored"},{"type":"text","text":"question"}]},{"role":"assistant","content":"answer"}]}`,
			want:   "system: rules\nuser: question\nassistant: answer\n",
		},
		{
			name:   "Responses plain input",
			format: types.RelayFormatOpenAIResponses,
			body:   `{"input":"question"}`,
			want:   "user: question\n",
		},
		{
			name:   "Responses items without function outputs or images",
			format: types.RelayFormatOpenAIResponses,
			body:   `{"instructions":"rules","input":[{"role":"user","content":[{"type":"input_image","image_url":"ignored"},{"type":"input_text","text":"question"}]},{"type":"function_call_output","output":"ignored"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]}]}`,
			want:   "system: rules\nuser: question\nassistant: answer\n",
		},
		{
			name:   "Gemini system and parts without thoughts",
			format: types.RelayFormatGemini,
			body:   `{"systemInstruction":{"parts":[{"text":"rules"}]},"contents":[{"role":"user","parts":[{"text":"question"}]},{"role":"model","parts":[{"text":"ignored","thought":true},{"text":"answer"}]}]}`,
			want:   "system: rules\nuser: question\nmodel: answer\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storage, err := common.CreateBodyStorage([]byte(test.body))
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, storage.Close()) })

			text, err := modelTextInput(storage, test.format, 64)
			require.NoError(t, err)
			assert.Equal(t, test.want, text)
		})
	}
}
