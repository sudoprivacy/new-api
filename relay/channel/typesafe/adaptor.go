// sudoapi: Typesafe adaptor.

package typesafe

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
)

var errUnsupported = errors.New("TypeSafe supports only /v1/systemone decisions")

type Adaptor struct {
	request *dto.TypesafeRequest
}

// Init requires no channel-specific setup; conversion captures the final request.
func (a *Adaptor) Init(*relaycommon.RelayInfo) {}

// GetRequestURL maps decisions to the native TypeSafe endpoint and rejects other modes.
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return strings.TrimRight(info.ChannelBaseUrl, "/") + "/v1/systemone", nil
}

// SetupRequestHeader applies shared headers and authenticates TypeSafe JSON requests.
func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	header.Set("Authorization", "Bearer "+info.ApiKey)
	header.Set("Content-Type", "application/json")
	return nil
}

// DoRequest sends the converted body through the shared channel transport.
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, body)
}

// DoResponse validates a bounded response before forwarding its original JSON and
// returning upstream token usage for settlement. Invalid answers or usage fail closed.
func (a *Adaptor) DoResponse(c *gin.Context, response *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	// Decisions contain compact scores, not arbitrary generated documents.
	const maxResponseBytes = 16 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return nil, types.NewOpenAIError(errors.New("failed to read TypeSafe response"), types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
	}
	var result dto.TypesafeResponse
	if err = common.Unmarshal(body, &result); err != nil {
		// Never estimate a successful charge from a missing or malformed usage.
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	usage := &dto.Usage{
		PromptTokens:     int(result.Usage.InputTokens),
		CompletionTokens: int(result.Usage.OutputTokens),
		TotalTokens:      int(result.Usage.InputTokens + result.Usage.OutputTokens),
	}
	// Preserve probabilities, confidence, legends and future upstream fields.
	info.SetFirstResponseTime()
	c.Data(http.StatusOK, "application/json", body)
	return usage, nil
}

func (a *Adaptor) GetModelList() []string { return []string{"jev-1.13.0", "jev-latest", "jev-preview"} }

// GetChannelName identifies the TypeSafe provider in the shared adaptor interface.
func (a *Adaptor) GetChannelName() string { return "typesafe" }

// ConvertOpenAIRequest rejects chat requests because TypeSafe requires native decisions.
func (a *Adaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errUnsupported
}

// ConvertRerankRequest rejects reranking, which this decisions adaptor does not support.
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errUnsupported
}

// ConvertEmbeddingRequest rejects embedding requests for this decisions-only provider.
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errUnsupported
}

// ConvertAudioRequest rejects audio requests for this decisions-only provider.
func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errUnsupported
}

// ConvertImageRequest rejects image requests for this decisions-only provider.
func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, errUnsupported
}

// ConvertOpenAIResponsesRequest rejects the Responses protocol instead of translating it into decisions.
func (a *Adaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, errUnsupported
}

// ConvertClaudeRequest rejects the Claude protocol instead of translating it into decisions.
func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errUnsupported
}

// ConvertGeminiRequest rejects the Gemini protocol instead of translating it into decisions.
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errUnsupported
}
