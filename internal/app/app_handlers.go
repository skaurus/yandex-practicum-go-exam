package app

import (
	"encoding/base64"
	"io"
	"net/http"
	"time"

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

const (
	loginCookieName   = "whoami"
	loginCookieMaxAge = time.Duration(1e9 * 60 * 60 * 24 * 365) // seconds
)

func (runEnv Env) setAuthCookie(c *gin.Context, u *users.User) {
	loginBytes := []byte(u.Login)

	runEnv.setSignedCookie(
		c,
		loginCookieName, base64.StdEncoding.EncodeToString(loginBytes),
		int(loginCookieMaxAge.Seconds()), "/", false, true,
	)
}

func (runEnv Env) getUserFromCookie(c *gin.Context) (u *users.User) {
	logger := runEnv.Logger()

	found, encodedLogin := runEnv.getSignedCookie(c, loginCookieName)
	if !found || len(encodedLogin) == 0 {
		return
	}

	loginBytes, err := base64.StdEncoding.DecodeString(encodedLogin)
	if err != nil {
		logger.Error().Err(err).Msg("login cookie decode error")
		return
	}

	u, err = runEnv.users.GetByLogin(c, string(loginBytes))
	if err != nil {
		logger.Error().Err(err).Msg("db error")
	}

	return
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
		// Not 100% sure that this is the only reason to have an empty user.
		// But it is for sure the most probable one.
		c.String(http.StatusConflict, "login already taken")
		return
	}

	runEnv.setAuthCookie(c, user)

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
		// Not 100% sure that this is the only reason to have an empty user.
		// But it is for sure the most probable one.
		c.String(http.StatusUnauthorized, "wrong login or password")
		return
	}

	if ok := runEnv.users.CheckPassword(user, req.Password); !ok {
		c.String(http.StatusUnauthorized, "wrong login or password")
		return
	}

	runEnv.setAuthCookie(c, user)

	c.String(http.StatusOK, "")
}

func (runEnv Env) handlerSayMyName(c *gin.Context) {
	user := runEnv.getUserFromCookie(c)

	c.String(http.StatusOK, user.Login)
}
