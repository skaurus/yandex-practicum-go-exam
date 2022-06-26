package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (runEnv Env) handlerPing(c *gin.Context) {
	c.String(http.StatusOK, "")
}
