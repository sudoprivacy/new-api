// sudoapi: Official Seedance task adaptor.

package doubao

import (
	"bytes"
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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type SeedanceTaskAdaptor struct {
	taskcommon.BaseBilling
}

func (a *SeedanceTaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// 按 token 计费, 改写预扣费, 按补全算
	if _, hasRatioSetting, _ := ratio_setting.GetModelRatio(info.OriginModelName); hasRatioSetting {
		info.PriceData.Quota *= int(ratio_setting.GetCompletionRatio(info.OriginModelName))
		return nil
	}

	// 按次计费, 加其他倍率
	req, err := a.taskRequest(c)
	if err != nil {
		return nil
	}
	ratios := make(map[string]float64)
	if req.Frames != nil {
		ratios["seconds"] = float64(*req.Frames) / 24
	} else if req.Duration != nil {
		ratios["seconds"] = float64(*req.Duration)
	} else {
		ratios["seconds"] = 5
	}
	return ratios
}

// AdjustBillingOnComplete returns 0 (keep pre-charged amount).
func (*SeedanceTaskAdaptor) AdjustBillingOnComplete(task *model.Task, info *relaycommon.TaskInfo) int {
	modelName := TaskModelName(task)
	// 获取模型价格和倍率
	modelRatio, hasRatioSetting, _ := ratio_setting.GetModelRatio(modelName)
	// 只有配置了倍率(非固定价格)时才按 token 重新计费
	if !hasRatioSetting || modelRatio <= 0 {
		return 0
	}

	// 获取用户和组的倍率信息
	group := task.Group
	if group == "" {
		user, err := model.GetUserById(task.UserId, false)
		if err == nil {
			group = user.Group
		}
	}
	if group == "" {
		return 0
	}

	groupRatio := ratio_setting.GetGroupRatio(group)
	userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(group, group)

	var finalGroupRatio float64
	if hasUserGroupRatio {
		finalGroupRatio = userGroupRatio
	} else {
		finalGroupRatio = groupRatio
	}

	inputTokens := float64(info.TotalTokens - info.CompletionTokens)
	outputTokens := float64(info.CompletionTokens)

	quota := (inputTokens + outputTokens*ratio_setting.GetCompletionRatio(modelName)) * modelRatio * finalGroupRatio
	return int(quota)
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
	if info.UpstreamModelName != "" {
		req.Model = info.UpstreamModelName
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
