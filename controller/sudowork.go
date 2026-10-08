// sudoapi: API for sudowork

package controller

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
	v1 "github.com/QuantumNous/new-api/controller/v1"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// UpdateUserQuota PUT /api/user/quota
// see ManageUser add_quota
// 兼容旧的接口, 供 sudowork 增减余额, 同时留下 comment
func UpdateUserQuota(ctx *gin.Context) {
	var req struct {
		ID      int    `json:"id"`
		Quota   int    `json:"quota"`
		Comment string `json:"comment"`
	}
	err := ctx.ShouldBindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusOK, gin.H{"success": false, "message": "Invalid parameter"})
		return
	}
	quota := req.Quota
	if quota > 0 {
		if err = model.IncreaseUserQuota(req.ID, quota, true); err != nil {
			common.ApiError(ctx, err)
			return
		}
		recordManageAuditFor(ctx, req.ID, "user.quota_add", map[string]any{
			"target_user_id": req.ID,
			"quota":          logger.LogQuota(quota),
			"comment":        req.Comment,
		})
	} else {
		if err := model.DecreaseUserQuota(req.ID, -quota, true); err != nil {
			common.ApiError(ctx, err)
			return
		}
		recordManageAuditFor(ctx, req.ID, "user.quota_subtract", map[string]any{
			"target_user_id": req.ID,
			"quota":          logger.LogQuota(-quota),
			"comment":        req.Comment,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
	return
}

// GetUserTokens GET /api/user/tokens
// 供 sudowork 获取用户 token
func GetUserTokens(ctx *gin.Context) {
	var opt model.QueryUserTokenOptions
	if err := ctx.ShouldBindQuery(&opt); err != nil {
		common.ApiError(ctx, v1.ValidationError(err))
		return
	}
	tokens, count, err := model.QueryUserTokens(opt)
	if err != nil {
		common.ApiErrorMsg(ctx, fmt.Sprintf("query user tokens failed, err: %v", err))
		return
	}
	common.ApiSuccess(ctx, gin.H{"data": buildMaskedTokenResponses(tokens), "count": count})
}

// UpdateUserTokenStatus PUT /api/user/token/status
// 供 sudowork 启用/禁用 用户 token
func UpdateUserTokenStatus(ctx *gin.Context) {
	var req struct {
		ID      int    `json:"id"`
		Status  string `json:"status"`
		Comment string `json:"comment"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusOK, gin.H{"success": false, "message": "Invalid parameter"})
		return
	}

	token, err := model.GetTokenById(req.ID)
	if err != nil {
		common.ApiError(ctx, err)
		return
	}

	switch req.Status {
	case "enable":
		if token.Status == common.TokenStatusExpired && token.ExpiredTime <= common.GetTimestamp() && token.ExpiredTime != -1 {
			common.ApiErrorI18n(ctx, i18n.MsgTokenExpiredCannotEnable)
			return
		}
		if token.Status == common.TokenStatusExhausted && token.RemainQuota <= 0 && !token.UnlimitedQuota {
			common.ApiErrorI18n(ctx, i18n.MsgTokenExhaustedCannotEable)
			return
		}
		token.Status = common.TokenStatusEnabled
	case "disabled":
		token.Status = common.TokenStatusDisabled
	default:
		common.ApiErrorMsg(ctx, fmt.Sprintf("invalid status: %s", req.Status))
		return
	}

	err = token.Update()
	if err != nil {
		common.ApiError(ctx, err)
		return
	}
	recordManageAuditFor(ctx, token.UserId, "user.token_status", map[string]any{
		"target_user_id":  token.UserId,
		"target_token_id": req.ID,
		"status":          req.Status,
		"comment":         req.Comment,
	})
	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(token),
	})
}

// UpdateUserTokenQuota PUT /api/user/token/quota
// 供 sudowork 增减用户 token 余额
func UpdateUserTokenQuota(ctx *gin.Context) {
	var req struct {
		ID             int    `json:"id"`
		RemainQuota    int    `json:"remain_quota"`
		DeltaQuota     int    `json:"delta_quota"`
		UnlimitedQuota bool   `json:"unlimited_quota"`
		Comment        string `json:"comment"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusOK, gin.H{"success": false, "message": "Invalid parameter"})
		return
	}

	token, err := model.GetTokenById(req.ID)
	if err != nil {
		common.ApiError(ctx, err)
		return
	}

	params := map[string]any{
		"target_user_id":  token.UserId,
		"target_token_id": req.ID,
		"comment":         req.Comment,
	}

	if !req.UnlimitedQuota {
		var remainQuota int
		if req.DeltaQuota != 0 {
			// 优先使用增减
			remainQuota = token.RemainQuota + req.DeltaQuota
		} else {
			// 次之使用覆盖
			remainQuota = req.RemainQuota
		}
		if remainQuota < 0 {
			common.ApiErrorI18n(ctx, i18n.MsgTokenQuotaNegative)
			return
		}
		if maxQuotaValue := maxTokenQuota(); remainQuota > maxQuotaValue {
			common.ApiErrorI18n(ctx, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
		token.RemainQuota = remainQuota
		params["remain_quota"] = remainQuota
	}
	token.UnlimitedQuota = req.UnlimitedQuota
	params["unlimited_quota"] = req.UnlimitedQuota

	if err = token.Update(); err != nil {
		common.ApiError(ctx, err)
		return
	}
	recordManageAuditFor(ctx, token.UserId, "user.token_quota", params)
	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(token),
	})
}
