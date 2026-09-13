package dto

import "encoding/json"

// Gemini Live WebSocket protocol DTOs
// Ref: https://ai.google.dev/api/live

// --- Client → Server messages ---

// GeminiLiveSetup is the mandatory first message from client to server.
type GeminiLiveSetup struct {
	Setup *GeminiLiveSetupConfig `json:"setup"`
}

type GeminiLiveSetupConfig struct {
	Model             string                      `json:"model"`
	GenerationConfig  *GeminiChatGenerationConfig `json:"generationConfig,omitempty"`
	SystemInstruction *GeminiChatContent          `json:"systemInstruction,omitempty"`
	Tools             json.RawMessage             `json:"tools,omitempty"`
	ToolConfig        *ToolConfig                 `json:"toolConfig,omitempty"`
}

// GeminiLiveClientMessage is a union type for all client→server messages after setup.
type GeminiLiveClientMessage struct {
	Setup         *GeminiLiveSetupConfig     `json:"setup,omitempty"`
	RealtimeInput *GeminiRealtimeInput       `json:"realtimeInput,omitempty"`
	ClientContent *GeminiClientContent       `json:"clientContent,omitempty"`
	ToolResponse  *GeminiLiveToolResponseMsg `json:"toolResponse,omitempty"`
}

type GeminiRealtimeInput struct {
	MediaChunks []GeminiMediaChunk `json:"mediaChunks,omitempty"`
}

type GeminiMediaChunk struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type GeminiClientContent struct {
	Turns        []GeminiChatContent `json:"turns,omitempty"`
	TurnComplete bool                `json:"turnComplete,omitempty"`
}

type GeminiLiveToolResponseMsg struct {
	FunctionResponses []GeminiFunctionResponse `json:"functionResponses"`
}

// --- Server → Client messages ---

// GeminiLiveServerMessage is a union type for all server→client messages.
type GeminiLiveServerMessage struct {
	SetupComplete           json.RawMessage          `json:"setupComplete,omitempty"`
	ServerContent           *GeminiLiveServerContent `json:"serverContent,omitempty"`
	ToolCall                *GeminiLiveToolCall      `json:"toolCall,omitempty"`
	ToolCallCancellation    json.RawMessage          `json:"toolCallCancellation,omitempty"`
	GoAway                  json.RawMessage          `json:"goAway,omitempty"`
	SessionResumptionUpdate json.RawMessage          `json:"sessionResumptionUpdate,omitempty"`
	UsageMetadata           *GeminiUsageMetadata     `json:"usageMetadata,omitempty"`
}

type GeminiLiveServerContent struct {
	ModelTurn    *GeminiChatContent `json:"modelTurn,omitempty"`
	TurnComplete bool               `json:"turnComplete,omitempty"`
	Interrupted  bool               `json:"interrupted,omitempty"`
}

type GeminiLiveToolCall struct {
	FunctionCalls []FunctionCall `json:"functionCalls"`
}
