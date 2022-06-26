package app

import (
	"io"
	"net/http"

	"github.com/skaurus/yandex-practicum-go-exam/internal/users"

	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func (runEnv Env) parseUserRequest(c *gin.Context) (ok bool, req users.Request) {
	logger := runEnv.Logger()

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.Error().Err(err).Msg("can't read request body")
		c.String(http.StatusBadRequest, err.Error())
		return
	}
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

	return true, req
}

func (runEnv Env) handlerUserRegister(c *gin.Context) {
	logger := runEnv.Logger()

	ok, req := runEnv.parseUserRequest(c)
	if !ok {
		return
	}

	user, err := runEnv.users.Create(c, req)
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, "db error")
		return
	}
	if user.ID == 0 {
		// Не 100% уверен, что других причин пустого юзера не может быть...
		// Но скорее всего дело в этом
		c.String(http.StatusConflict, "login already taken")
		return
	}

	c.String(http.StatusOK, "")
}

func (runEnv Env) handlerUserLogin(c *gin.Context) {
	logger := runEnv.Logger()

	ok, req := runEnv.parseUserRequest(c)
	if !ok {
		return
	}

	user, err := runEnv.users.GetByLogin(c, req.Login)
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, "db error")
		return
	}
	if user.ID == 0 {
		// Не 100% уверен, что других причин пустого юзера не может быть...
		// Но скорее всего дело в этом. Ошибка специально общая для логина и пароля
		c.String(http.StatusUnauthorized, "wrong login or password")
		return
	}

	if ok := runEnv.users.CheckPassword(user, req.Password); !ok {
		c.String(http.StatusUnauthorized, "wrong login or password")
		return
	}

	c.String(http.StatusOK, "")
}

func (runEnv Env) handlerPing(c *gin.Context) {
	c.String(http.StatusOK, "")
}
