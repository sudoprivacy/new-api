// sudoapi: Official Seedance task adaptor.

package doubao

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
)

type SeedanceTaskAdaptor struct {
	taskcommon.BaseBilling
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

func (a *SeedanceTaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if strings.HasSuffix(info.ChannelBaseUrl, "/contents/generations/tasks") {
		return info.ChannelBaseUrl, nil
	}
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", info.ChannelBaseUrl), nil
}

func (a *SeedanceTaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *SeedanceTaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
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
	var payload responsePayload
	if err = common.Unmarshal(body, &payload); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", body), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if payload.ID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}

	c.JSON(http.StatusOK, gin.H{"id": info.PublicTaskID})
	return payload.ID, body, nil
}

func (a *SeedanceTaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	var url string
	if strings.HasSuffix(baseUrl, "/contents/generations/tasks") {
		url = fmt.Sprintf("%s/%s", baseUrl, taskID)
	} else {
		url = fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)
	}

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
	return new(TaskAdaptor).ParseTaskResult(respBody)
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
