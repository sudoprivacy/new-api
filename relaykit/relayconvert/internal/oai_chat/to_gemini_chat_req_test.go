// sudoapi: bypass gemini thoughtSignature when channel changes.

package oaichat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	sharedgemini "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/gemini"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

type MockGinContext struct {
	context.Context
	changed bool
}

func (c MockGinContext) GetBool(key string) bool {
	return key == "retry_channel_changed" && c.changed
}

func TestOpenAIChatRequestToGeminiGenerateContentThoughtSignature(t *testing.T) {
	tests := []struct {
		name            string
		ctx             context.Context
		extraContent    json.RawMessage
		fallbackEnabled bool
		wantSignature   string
	}{
		{
			name:          "original signature without retry context",
			ctx:           context.Background(),
			extraContent:  json.RawMessage(`{"google":{"thought_signature":"original"}}`),
			wantSignature: "original",
		},
		{
			name:          "original signature when channel is unchanged",
			ctx:           MockGinContext{Context: context.Background()},
			extraContent:  json.RawMessage(`{"google":{"thought_signature":"original"}}`),
			wantSignature: "original",
		},
		{
			name:          "changed channel replaces original signature",
			ctx:           MockGinContext{Context: context.Background(), changed: true},
			extraContent:  json.RawMessage(`{"google":{"thought_signature":"original"}}`),
			wantSignature: sharedgemini.ThoughtSignatureBypassValue,
		},
		{
			name:          "changed channel adds bypass without original signature",
			ctx:           MockGinContext{Context: context.Background(), changed: true},
			wantSignature: sharedgemini.ThoughtSignatureBypassValue,
		},
		{
			name:          "missing signature stays omitted when unchanged",
			ctx:           MockGinContext{Context: context.Background()},
			wantSignature: "",
		},
		{
			name:            "configured fallback adds bypass when unchanged",
			ctx:             MockGinContext{Context: context.Background()},
			fallbackEnabled: true,
			wantSignature:   sharedgemini.ThoughtSignatureBypassValue,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolCalls, err := kitutil.Marshal([]dto.ToolCallRequest{{
				ID:           "call_1",
				Type:         "function",
				Function:     dto.FunctionRequest{Name: "lookup", Arguments: `{"q":"x"}`},
				ExtraContent: tt.extraContent,
			}})
			require.NoError(t, err)
			request := dto.GeneralOpenAIRequest{
				Model:    "gemini-test",
				Messages: []dto.Message{{Role: "assistant", ToolCalls: toolCalls}},
			}
			info := &convmeta.Values{Options: &convmeta.Options{Gemini: convmeta.GeminiOptions{
				FunctionCallThoughtSignatureEnabled: tt.fallbackEnabled,
			}}}
			got, err := OpenAIChatRequestToGeminiGenerateContent(tt.ctx, request, info)
			require.NoError(t, err)
			require.Len(t, got.Contents, 1)
			require.Len(t, got.Contents[0].Parts, 1)
			part := got.Contents[0].Parts[0]
			require.NotNil(t, part.FunctionCall)
			assert.Equal(t, "lookup", part.FunctionCall.FunctionName)

			if tt.wantSignature == "" {
				assert.Empty(t, part.ThoughtSignature)
			} else {
				var signature string
				require.NoError(t, kitutil.Unmarshal(part.ThoughtSignature, &signature))
				assert.Equal(t, tt.wantSignature, signature)
			}
		})
	}
}
