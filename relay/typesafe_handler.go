// sudoapi: Typesafe adaptor.

package relay

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/typesafe"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
)

func TypesafeHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	info.InitChannelMeta(c)
	typesafeReq, ok := info.Request.(*dto.TypesafeRequest)
	if !ok {
		return types.NewError(errors.New("invalid typesafe request"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	request, err := common.DeepCopy(typesafeReq)
	if err != nil {
		return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if err = helper.ModelMappedHelper(c, info, request); err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	var adaptor typesafe.Adaptor
	adaptor.Init(info)
	jsonData, err := common.Marshal(request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer func() { _ = closer.Close() }()
	response, err := adaptor.DoRequest(c, info, body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
	}
	httpResponse, ok := response.(*http.Response)
	if !ok || httpResponse == nil {
		return types.NewError(errors.New("missing upstream response"), types.ErrorCodeBadResponseBody)
	}
	defer func() { _ = httpResponse.Body.Close() }()
	statusMapping := c.GetString("status_code_mapping")
	if httpResponse.StatusCode != http.StatusOK {
		apiErr := service.RelayErrorHandler(c.Request.Context(), httpResponse, false)
		service.ResetStatusCode(apiErr, statusMapping)
		return apiErr
	}
	usage, apiErr := adaptor.DoResponse(c, httpResponse, info)
	if apiErr != nil {
		service.ResetStatusCode(apiErr, statusMapping)
		return apiErr
	}
	service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	return nil
}
