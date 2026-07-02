// sudoapi: Official Seedance task adaptor.

package doubao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

type SeedanceTaskAdaptor struct {
	taskcommon.BaseBilling
}

func (a *SeedanceTaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	info.PriceData.Quota = info.TieredBillingSnapshot.EstimatedQuotaAfterGroup
	return nil
}

func (a *SeedanceTaskAdaptor) AdjustBillingOnCompleteExt(task *model.Task, info *relaycommon.TaskInfo) (service.AdjustResult, bool) {
	if task.PrivateData.BillingContext == nil {
		return service.AdjustResult{}, false
	}
	snap := task.PrivateData.BillingContext.TieredBillingSnapshot
	if snap == nil || snap.ExprString == "" || snap.EstimatedTier == "" {
		return service.AdjustResult{}, false
	}
	trace, err := runExpr(snap.ExprString, requestPayload{}, snap.EstimatedTier, info.CompletionTokens)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("tiered expr run failed, task: %s, err: %v", task.TaskID, err))
		return service.AdjustResult{}, false
	}
	if trace.MatchedTier == "" || trace.Cost == 0 {
		logger.LogWarn(context.Background(), fmt.Sprintf("tiered expr not match, task: %s, model: %s", task.TaskID, task.Properties.OriginModelName))
		return service.AdjustResult{}, false
	}
	quota, clamp := common.QuotaFromFloatChecked(trace.Cost / 100_0000 * common.QuotaPerUnit * snap.GroupRatio)
	return service.AdjustResult{Quota: quota, QuotaClamp: clamp}, true
}

// AdjustBillingOnComplete 是 AdjustBillingOnCompleteExt 的 fallback
func (a *SeedanceTaskAdaptor) AdjustBillingOnComplete(task *model.Task, info *relaycommon.TaskInfo) int {
	return int(float64(task.Quota) / float64(estimatedCompletionTokens) * float64(info.CompletionTokens))
}

func (a *SeedanceTaskAdaptor) GetModelList() []string { return ModelList }

func (a *SeedanceTaskAdaptor) GetChannelName() string { return "seedance" }

func (a *SeedanceTaskAdaptor) Init(*relaycommon.RelayInfo) {}

func (a *SeedanceTaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	var req requestPayload
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if len(req.Content) == 0 {
		return service.TaskErrorWrapperLocal(errors.New("content is required"), "invalid_request", http.StatusBadRequest)
	}
	if req.Resolution == "" {
		req.Resolution = "720p"
	} else {
		req.Resolution = strings.ToLower(req.Resolution)
	}
	snap, err := getSeedanceTieredBillingSnapshot(c, info, req)
	if err != nil {
		return err
	}
	info.TieredBillingSnapshot = snap

	info.Action = constant.TaskActionGenerate
	c.Set("task_request", req)
	return nil
}

func (a *SeedanceTaskAdaptor) requestURL(url string) string {
	if strings.HasSuffix(url, "/contents/generations/tasks") {
		return url
	}
	if strings.HasSuffix(url, "/v1/video/tasks") {
		return url
	}
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", url)
}

func (a *SeedanceTaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return a.requestURL(info.ChannelBaseUrl), nil
}

func (a *SeedanceTaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *SeedanceTaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := a.taskRequest(c)
	if err != nil {
		return nil, err
	}
	if upstreamModelName := info.GetUpstreamModelName(); upstreamModelName != "" {
		req.Model = upstreamModelName
	}
	data, err := common.Marshal(req)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *SeedanceTaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *SeedanceTaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	var payload struct {
		ID     string `json:"id"`
		TaskID string `json:"task_id"`
	}
	if err = common.Unmarshal(body, &payload); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", body), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	taskID := lo.Ternary(payload.ID != "", payload.ID, payload.TaskID)
	if taskID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	c.JSON(http.StatusOK, gin.H{"id": info.PublicTaskID})
	return taskID, body, nil
}

func (a *SeedanceTaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	url := fmt.Sprintf("%s/%s", a.requestURL(baseUrl), taskID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *SeedanceTaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var response SeedanceResponse
	if err := common.Unmarshal(respBody, &response); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	var taskResult relaycommon.TaskInfo
	switch response.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = response.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = response.Usage.CompletionTokens
		taskResult.TotalTokens = response.Usage.TotalTokens
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = response.Error.Message
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusUnknown
		taskResult.Progress = "30%"
	}
	return &taskResult, nil
}

func (a *SeedanceTaskAdaptor) taskRequest(c *gin.Context) (*requestPayload, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(requestPayload)
	if !ok {
		return nil, fmt.Errorf("invalid task request type")
	}
	return &req, nil
}

func (a *SeedanceTaskAdaptor) ConvertToSeedanceVideo(originTask *model.Task) ([]byte, error) {
	var status string
	switch originTask.Status {
	case model.TaskStatusQueued:
		status = "queued"
	case model.TaskStatusInProgress:
		status = "running"
	case model.TaskStatusSuccess:
		status = "succeeded"
	case model.TaskStatusFailure:
		status = "failed"
	default:
		status = gjson.GetBytes(originTask.Data, "status").String()
	}
	return json.Marshal(SeedanceResponse{
		ID:     originTask.TaskID,
		Model:  TaskModelName(originTask),
		Status: status,
		Content: SeedanceResponseContent{
			VideoURL: originTask.GetResultURL(),
		},
		Usage: SeedanceResponseUsage{
			CompletionTokens: int(gjson.GetBytes(originTask.Data, "usage.completion_tokens").Int()),
			TotalTokens:      int(gjson.GetBytes(originTask.Data, "usage.total_tokens").Int()),
		},
		Error: SeedanceResponseError{
			Code:    gjson.GetBytes(originTask.Data, "error.code").String(),
			Message: gjson.GetBytes(originTask.Data, "error.message").String(),
		},
		CreatedAt: originTask.CreatedAt,
		UpdatedAt: originTask.UpdatedAt,
	})
}

type (
	SeedanceResponse struct {
		ID        string                  `json:"id"`
		Model     string                  `json:"model"`
		Status    string                  `json:"status"`
		Content   SeedanceResponseContent `json:"content"`
		Usage     SeedanceResponseUsage   `json:"usage"`
		Error     SeedanceResponseError   `json:"error,omitempty"`
		CreatedAt int64                   `json:"created_at"`
		UpdatedAt int64                   `json:"updated_at"`
	}
	SeedanceResponseContent struct {
		VideoURL string `json:"video_url"`
	}
	SeedanceResponseUsage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	}
	SeedanceResponseError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
)

func TaskModelName(task *model.Task) string {
	if bc := task.PrivateData.BillingContext; bc != nil && bc.OriginModelName != "" {
		return bc.OriginModelName
	}
	return task.Properties.OriginModelName
}
