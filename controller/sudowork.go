// sudoapi: API for sudowork

package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/common"
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
