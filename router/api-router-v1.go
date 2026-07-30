// sudoapi: Logs api.

package router

import (
	"github.com/gin-gonic/gin"

	v1 "github.com/QuantumNous/new-api/controller/v1"
	"github.com/QuantumNous/new-api/middleware"
)

func setupApiV1Router(apiRouter gin.IRouter) {
	v1router := apiRouter.Group("/v1")

	{
		logGroup := v1router.Group("/logs")
		logGroup.Use(middleware.TokenAuthReadOnly())
		logGroup.GET("/", v1.QueryUserLogs)
	}
}
