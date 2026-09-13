package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ClientMessageRewriter is an optional function to rewrite client messages before forwarding to upstream.
// Used by Vertex AI to rewrite the model field in setup messages.
type ClientMessageRewriter func(message []byte) []byte

// safeCountTextToken wraps service.CountTextToken with panic recovery
// for environments where the tokenizer is not initialized (e.g. tests).
func safeCountTextToken(text, model string) (tokens int) {
	defer func() {
		if r := recover(); r != nil {
			tokens = 0
		}
	}()
	return service.CountTextToken(text, model)
}

// GeminiLiveRealtimeHandler handles bidirectional WebSocket relay for Gemini Live API.
// It follows the same dual-goroutine pattern as OpenaiRealtimeHandler.
// Optional rewriters can modify client messages before forwarding (e.g. Vertex model name rewrite).
func GeminiLiveRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo, rewriters ...ClientMessageRewriter) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	errChan := make(chan error, 2)

	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}

	// Goroutine 1: Client → Upstream (transparent relay with token counting)
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}

				// Apply client message rewriters (e.g. Vertex model name rewrite)
				for _, rewrite := range rewriters {
					if rewrite != nil {
						message = rewrite(message)
					}
				}

				// Count input tokens
				textTokens, audioTokens := countGeminiLiveClientTokens(info, message)
				if textTokens+audioTokens > 0 {
					localUsage.TotalTokens += textTokens + audioTokens
					localUsage.InputTokens += textTokens + audioTokens
					localUsage.InputTokenDetails.TextTokens += textTokens
					localUsage.InputTokenDetails.AudioTokens += audioTokens
					logger.LogInfo(c, fmt.Sprintf("gemini live client: textTokens=%d, audioTokens=%d", textTokens, audioTokens))
				}

				// Forward to upstream
				err = helper.WssString(c, targetConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}
			}
		}
	})

	// Goroutine 2: Upstream → Client (transparent relay with token counting + billing)
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()
		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()

				var serverMsg dto.GeminiLiveServerMessage
				_ = json.Unmarshal(message, &serverMsg)

				// Count output tokens from server content
				if serverMsg.ServerContent != nil {
					textTokens, audioTokens := countGeminiLiveServerTokens(info, &serverMsg)
					localUsage.TotalTokens += textTokens + audioTokens
					localUsage.OutputTokens += textTokens + audioTokens
					localUsage.OutputTokenDetails.TextTokens += textTokens
					localUsage.OutputTokenDetails.AudioTokens += audioTokens

					if textTokens+audioTokens > 0 {
						logger.LogInfo(c, fmt.Sprintf("gemini live server: textTokens=%d, audioTokens=%d", textTokens, audioTokens))
					}

					// Billing trigger: on turnComplete (equivalent to OpenAI's response.done)
					if serverMsg.ServerContent.TurnComplete {
						logger.LogInfo(c, fmt.Sprintf("gemini live turnComplete, localUsage: %+v", localUsage))
						_ = geminiPreConsumeUsage(c, info, localUsage, sumUsage)
						localUsage = &dto.RealtimeUsage{}
					}
				}

				// Count tool call tokens
				if serverMsg.ToolCall != nil {
					textTokens := countGeminiLiveToolCallTokens(info, serverMsg.ToolCall)
					localUsage.TotalTokens += textTokens
					localUsage.OutputTokens += textTokens
					localUsage.OutputTokenDetails.TextTokens += textTokens
				}

				// If Gemini provides usage metadata, log it
				if serverMsg.UsageMetadata != nil {
					logger.LogInfo(c, fmt.Sprintf("gemini live usageMetadata: prompt=%d, candidates=%d, total=%d",
						serverMsg.UsageMetadata.PromptTokenCount,
						serverMsg.UsageMetadata.CandidatesTokenCount,
						serverMsg.UsageMetadata.TotalTokenCount))
				}

				// Forward to client
				err = helper.WssString(c, clientConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}
			}
		}
	})

	// Wait for completion
	select {
	case <-clientClosed:
	case <-targetClosed:
	case err := <-errChan:
		logger.LogError(c, "gemini live error: "+err.Error())
	case <-c.Done():
	}

	// Flush remaining usage
	if localUsage.TotalTokens != 0 {
		_ = geminiPreConsumeUsage(c, info, localUsage, sumUsage)
	}

	logger.LogInfo(c, fmt.Sprintf("gemini live session ended, sumUsage: %+v", sumUsage))
	return nil, sumUsage
}

// geminiPreConsumeUsage accumulates usage and triggers quota consumption.
func geminiPreConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens

	err := service.PreWssConsumeQuota(ctx, info, usage)
	return err
}

// countGeminiLiveClientTokens counts input tokens from a client message.
func countGeminiLiveClientTokens(info *relaycommon.RelayInfo, message []byte) (textTokens int, audioTokens int) {
	var clientMsg dto.GeminiLiveClientMessage
	if err := json.Unmarshal(message, &clientMsg); err != nil {
		return 0, 0
	}

	// Setup message: count system instruction tokens
	if clientMsg.Setup != nil {
		if clientMsg.Setup.SystemInstruction != nil {
			for _, part := range clientMsg.Setup.SystemInstruction.Parts {
				if part.Text != "" {
					textTokens += safeCountTextToken(part.Text, info.UpstreamModelName)
				}
			}
		}
		return
	}

	// realtimeInput: audio/video media chunks
	if clientMsg.RealtimeInput != nil {
		for _, chunk := range clientMsg.RealtimeInput.MediaChunks {
			if strings.HasPrefix(chunk.MimeType, "audio/") {
				atk, err := service.CountAudioTokenInput(chunk.Data, "pcm16")
				if err == nil {
					audioTokens += atk
				}
			}
			// Video frames: estimate as image tokens (258 per frame)
			if strings.HasPrefix(chunk.MimeType, "image/") {
				textTokens += 258
			}
		}
		return
	}

	// clientContent: text turns
	if clientMsg.ClientContent != nil {
		for _, turn := range clientMsg.ClientContent.Turns {
			for _, part := range turn.Parts {
				if part.Text != "" {
					textTokens += safeCountTextToken(part.Text, info.UpstreamModelName)
				}
			}
		}
		return
	}

	// toolResponse: count response content as text tokens
	if clientMsg.ToolResponse != nil {
		for _, resp := range clientMsg.ToolResponse.FunctionResponses {
			if resp.Response != nil {
				data, err := json.Marshal(resp.Response)
				if err == nil {
					textTokens += safeCountTextToken(string(data), info.UpstreamModelName)
				}
			}
		}
		return
	}

	return
}

// countGeminiLiveServerTokens counts output tokens from a server content message.
func countGeminiLiveServerTokens(info *relaycommon.RelayInfo, msg *dto.GeminiLiveServerMessage) (textTokens int, audioTokens int) {
	if msg.ServerContent == nil || msg.ServerContent.ModelTurn == nil {
		return 0, 0
	}

	for _, part := range msg.ServerContent.ModelTurn.Parts {
		if part.Text != "" {
			textTokens += safeCountTextToken(part.Text, info.UpstreamModelName)
		}
		if part.InlineData != nil && strings.HasPrefix(part.InlineData.MimeType, "audio/") {
			atk, err := service.CountAudioTokenOutput(part.InlineData.Data, "pcm16")
			if err == nil {
				audioTokens += atk
			}
		}
	}
	return
}

// countGeminiLiveToolCallTokens counts text tokens from tool call function names and arguments.
func countGeminiLiveToolCallTokens(info *relaycommon.RelayInfo, toolCall *dto.GeminiLiveToolCall) int {
	textTokens := 0
	for _, fc := range toolCall.FunctionCalls {
		textTokens += safeCountTextToken(fc.FunctionName, info.UpstreamModelName)
		if fc.Arguments != nil {
			data, err := json.Marshal(fc.Arguments)
			if err == nil {
				textTokens += safeCountTextToken(string(data), info.UpstreamModelName)
			}
		}
	}
	return textTokens
}
