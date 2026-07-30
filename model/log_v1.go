// sudoapi: Logs api.

package model

import (
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/QuantumNous/new-api/common"
)

// QueryUserLogDTOs 覆盖原本 GetUserLogs
// 此前 formatUserLogs 只简单删除敏感信息, 现在按白名单过滤
func QueryUserLogDTOs(opt QueryUserLogOptions) ([]*LogDTO, int64, error) {
	logs, count, err := queryUserLogs(opt)
	if err != nil {
		return nil, 0, err
	}
	return lo.Map(logs, func(log *Log, i int) *LogDTO {
		return &LogDTO{
			Id:                i + 1,
			CreatedAt:         log.CreatedAt,
			Type:              log.Type,
			Content:           log.Content,
			Username:          log.Username,
			TokenId:           log.TokenId,
			TokenName:         log.TokenName,
			ModelName:         log.ModelName,
			Quota:             log.Quota,
			PromptTokens:      log.PromptTokens,
			CompletionTokens:  log.CompletionTokens,
			UseTime:           log.UseTime,
			IsStream:          log.IsStream,
			Group:             log.Group,
			Ip:                log.Ip,
			RequestId:         log.RequestId,
			UpstreamRequestId: log.UpstreamRequestId,
			Other:             common.MapToJsonStr(filterOtherMap(log.Other)),
		}
	}), count, nil
}

func QueryUserLogs(opt QueryUserLogOptions) ([]*UserLog, int64, error) {
	logs, count, err := queryUserLogs(opt)
	if err != nil {
		return nil, 0, err
	}
	return lo.Map(logs, func(log *Log, _ int) *UserLog {
		typeName, ok := LogType2Name[log.Type]
		if !ok {
			typeName = "unknown"
		}
		return &UserLog{
			CreatedAt:        log.CreatedAt,
			Type:             typeName,
			APIKeyName:       log.TokenName,
			ModelName:        log.ModelName,
			Duration:         log.UseTime,
			PromptTokens:     log.PromptTokens,
			CompletionTokens: log.CompletionTokens,
			Cost:             log.Quota,
			Detail:           log.Content,
			Other:            filterOtherMap(log.Other),
		}
	}), count, nil
}

var (
	LogName2Type = map[string]int{
		"unknown":     LogTypeUnknown,
		"recharge":    LogTypeTopup,
		"consumption": LogTypeConsume,
		"manage":      LogTypeManage,
		"system":      LogTypeSystem,
		"error":       LogTypeError,
		"refund":      LogTypeRefund,
		"login":       LogTypeLogin,
	}
	LogType2Name = map[int]string{
		LogTypeUnknown: "unknown",
		LogTypeTopup:   "recharge",
		LogTypeConsume: "consumption",
		LogTypeManage:  "manage",
		LogTypeSystem:  "system",
		LogTypeError:   "error",
		LogTypeRefund:  "refund",
		LogTypeLogin:   "login",
	}

	allowedOrderFields = mapset.NewThreadUnsafeSet("created_at")

	allowedOtherFields = mapset.NewThreadUnsafeSet(
		logOtherContentPartsKey,

		// Gork 违规扣费
		"violation_fee", "violation_fee_code", "fee_quota", "base_amount", "group_ratio", "status_code",
		"upstream_error_type", "upstream_error_code", "violation_fee_marker",

		// token 相关
		"cache_tokens", "text_input", "text_output",
		// audio
		"audio",
		"audio_input", "audio_output",
		"audio_ratio", "audio_completion_ratio",
		"audio_input_seperate_price", "audio_input_token_count", "audio_input_price",
		// image
		"image",
		"image_count", "image_output", "image_ratio", "image_cache_tokens",
		// claude 相关
		"claude", "usage_semantic",
		"cache_creation_tokens", "cache_creation_ratio",
		"cache_creation_tokens_5m", "cache_creation_ratio_5m",
		"cache_creation_tokens_1h", "cache_creation_ratio_1h",
		"cache_write_tokens",

		// 价格相关
		"model_price", "model_ratio", "completion_ratio", "cache_ratio",
		"group_ratio", "user_group_ratio",

		// 账单相关
		"billing_mode", "expr_b64", "matched_tier",
		"billing_tokens", "billing_unit", "fixed_price", "request_rules",
		"billing_source", "billing_preference",
		"usage_facts",

		// 任务相关
		"is_task", "task_id", "reason", "task_sync",
		"result_discarded", "pre_consumed_quota", "actual_quota",

		// 请求统计相关
		"request_path", "request_conversion",
		"frt", "reasoning_effort", "stream_status",
		"ws",

		// 工具计费
		"tool_surcharges",
		// 以前的工具计费
		"web_search", "web_search_call_count", "web_search_price",
		"file_search", "file_search_call_count", "file_search_price",
		"image_generation_call", "image_generation_call_count", "image_generation_call_price",

		// 错误日志
		"error_type", "error_code", "status_code",

		// 已经废弃的
		"prices", "path", "x_request_id", "request_id", "properties",
		"cache_creation_5m_tokens", "cache_creation_1h_tokens", "cache_creation_5m_ratio", "cache_creation_1h_ratio",
	)
)

type (
	Paginator struct {
		PageSize int    `json:"page_size" form:"page_size"`
		PageNum  int    `json:"page_num" form:"page_num"`
		OrderBy  string `json:"order_by" form:"order_by"`
		Desc     bool   `json:"desc" form:"desc"`
	}
	QueryUserLogOptions struct {
		Paginator

		UserID int `json:"user_id" form:"user_id"`

		TypeName string `json:"type" form:"type" binding:"omitempty,oneof=recharge consumption manage system error refund login"`
		Type     int

		TimeFrom   int64  `json:"time_from" form:"time_from"`
		TimeTo     int64  `json:"time_to" form:"time_to"`
		ModelName  string `json:"model_name" form:"model_name"`
		ApiKeyName string `json:"api_key_name" form:"api_key_name"`
		Group      string `json:"group" form:"group"`

		RequestID         string `json:"request_id" form:"request_id"`
		UpstreamRequestID string `json:"upstream_request_id" form:"upstream_request_id"`
	}

	UserLog struct {
		CreatedAt        int64          `json:"created_at"`
		Type             string         `json:"type"`
		APIKeyName       string         `json:"api_key_name"`
		ModelName        string         `json:"model_name"`
		Duration         int            `json:"duration"`
		PromptTokens     int            `json:"prompt_tokens"`
		CompletionTokens int            `json:"completion_tokens"`
		Cost             int            `json:"cost"`
		Detail           string         `json:"detail"`
		Other            map[string]any `json:"other"`
	}
	LogDTO struct {
		Id                int    `json:"id"`
		CreatedAt         int64  `json:"created_at"`
		Type              int    `json:"type"`
		Content           string `json:"content"`
		Username          string `json:"username"`
		TokenId           int    `json:"token_id"`
		TokenName         string `json:"token_name"`
		ModelName         string `json:"model_name"`
		Quota             int    `json:"quota"`
		PromptTokens      int    `json:"prompt_tokens"`
		CompletionTokens  int    `json:"completion_tokens"`
		UseTime           int    `json:"use_time"`
		IsStream          bool   `json:"is_stream"`
		Group             string `json:"group"`
		Ip                string `json:"ip"`
		RequestId         string `json:"request_id,omitempty"`
		UpstreamRequestId string `json:"upstream_request_id,omitempty"`
		Other             string `json:"other"`
	}
)

func filterOtherMap(otherStr string) map[string]any {
	other, _ := common.StrToMap(otherStr)
	return lo.PickBy(other, func(k string, v any) bool { return allowedOtherFields.Contains(k) })
}

func queryUserLogs(opt QueryUserLogOptions) ([]*Log, int64, error) {
	query := LOG_DB.Model(&Log{})

	if opt.UserID != 0 {
		query = query.Where("user_id = ?", opt.UserID)
	}

	if opt.Type != 0 {
		query = query.Where("type = ?", opt.Type)
	} else if typ, ok := LogName2Type[opt.TypeName]; ok && typ != 0 {
		query = query.Where("type = ?", typ)
	}

	if opt.TimeFrom != 0 {
		query = query.Where("created_at >= ?", opt.TimeFrom)
	}
	if opt.TimeTo != 0 {
		query = query.Where("created_at <= ?", opt.TimeTo)
	}
	if opt.ModelName != "" {
		query = query.Where("model_name = ?", opt.ModelName)
	}
	if opt.ApiKeyName != "" {
		query = query.Where("token_name = ?", opt.ApiKeyName)
	}

	if allowedOrderFields.Contains(opt.OrderBy) {
		query = query.Order(clause.OrderByColumn{
			Column: clause.Column{Name: opt.OrderBy},
			Desc:   opt.Desc,
		})
	}

	return FindByPaginator[Log](query, opt.Paginator)
}

func FindByPaginator[T any](tx *gorm.DB, paginator Paginator) ([]*T, int64, error) {
	var count int64
	if err := tx.Offset(-1).Limit(-1).Count(&count).Error; err != nil {
		return nil, 0, err
	}

	pageSize, pageNum := paginator.PageSize, paginator.PageNum
	if pageSize == 0 || pageSize > 100 {
		pageSize = 20
	}
	offset := pageSize * max(pageNum-1, 0)
	limit := pageSize

	var data []*T
	if err := tx.Offset(offset).Limit(limit).Find(&data).Error; err != nil {
		return nil, 0, err
	}
	return data, count, nil
}
