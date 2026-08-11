// sudoapi: Quota in chat completions usage for sudowork.

package common

import (
	"github.com/gin-gonic/gin"
)

func IsSudoworkRequest(c *gin.Context) bool {
	return c.GetHeader("X-Sudowork") != ""
}
