// sudoapi: Logs api.

package v1

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	ven "github.com/go-playground/validator/v10/translations/en"
	"github.com/samber/lo"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

// GetUserSelfLogs GET /api/log/self 替代原本的 controller.GetUserLogs
// 对 others 中敏感信息进行过滤
func GetUserSelfLogs(c *gin.Context) {
	userID := c.GetInt(string(constant.ContextKeyUserId))
	if userID == 0 {
		common.ApiErrorMsg(c, "unauthorized")
		return
	}
	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	tokenName := c.Query("token_name")
	group := c.Query("group")
	requestID := c.Query("request_id")
	upstreamRequestID := c.Query("upstream_request_id")

	logs, count, err := model.QueryUserLogDTOs(model.QueryUserLogOptions{
		Paginator:         model.Paginator{PageSize: pageInfo.PageSize, PageNum: pageInfo.Page},
		UserID:            userID,
		TypeName:          "",
		Type:              logType,
		TimeFrom:          startTimestamp,
		TimeTo:            endTimestamp,
		ModelName:         modelName,
		ApiKeyName:        tokenName,
		Group:             group,
		RequestID:         requestID,
		UpstreamRequestID: upstreamRequestID,
	})
	if err != nil {
		logger.LogError(c, fmt.Sprintf("query user logs failed, err: %v", err))
		common.ApiErrorMsg(c, "query user logs failed")
		return
	}

	pageInfo.SetTotal(int(count))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

// QueryUserLogs GET /api/v1/logs/ 用户使用 appikey 进行日志查询
func QueryUserLogs(ctx *gin.Context) {
	var opt model.QueryUserLogOptions
	if err := ctx.ShouldBindQuery(&opt); err != nil {
		logger.LogError(ctx, fmt.Sprintf("error: %v\n", err))
		common.ApiError(ctx, ValidationError(err))
		return
	}

	userID := ctx.GetInt(string(constant.ContextKeyUserId))
	if userID == 0 {
		common.ApiErrorMsg(ctx, "unauthorized")
		return
	}
	opt.UserID = userID
	logs, count, err := model.QueryUserLogs(opt)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("query user logs failed, err: %v", err))
		common.ApiErrorMsg(ctx, "query user logs failed")
		return
	}

	common.ApiSuccess(ctx, gin.H{"data": logs, "count": count})
}

func ValidationError(err error) error {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return err
	}
	msg := lo.Map(ve, func(fe validator.FieldError, _ int) string {
		return fe.Translate(translator)
	})
	return errors.New(strings.Join(msg, ", "))
}

var translator ut.Translator

func init() {
	translator = ut.New(en.New()).GetFallback()
	validate := binding.Validator.Engine().(*validator.Validate)
	if err := ven.RegisterDefaultTranslations(validate, translator); err != nil {
		panic(err)
	}
}
