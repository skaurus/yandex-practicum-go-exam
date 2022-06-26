package app

import (
	"io"
	"net/http"

	"github.com/skaurus/yandex-practicum-go-exam/internal/users"

	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func (runEnv Env) handlerUserRegister(c *gin.Context) {
	logger := runEnv.Logger()

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.Error().Err(err).Msg("can't read request body")
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	var req users.Request
	err = json.Unmarshal(body, &req)
	if err != nil {
		logger.Error().Err(err).Msg("can't parse body")
		c.String(http.StatusBadRequest, "can't parse json")
		return
	}
	if len(req.Login) == 0 || len(req.Password) == 0 {
		logger.Warn().Msg("empty login or password")
		c.String(http.StatusBadRequest, "empty login or password")
		return
	}

	user, err := runEnv.users.Create(c, req)
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, "db error")
		return
	}
	if user.ID == 0 {
		// не 100% уверен, что других причин пустого юзера не может быть...
		// но скорее всего дело в этом
		c.String(http.StatusConflict, "login already taken")
		return
	}

	c.String(http.StatusOK, "")
}

func (runEnv Env) handlerPing(c *gin.Context) {
	c.String(http.StatusOK, "")
}
