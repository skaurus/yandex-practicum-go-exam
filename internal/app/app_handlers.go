package app

import (
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/users"

	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
	"github.com/shopspring/decimal"
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

func (runEnv Env) handlerSayMyName(c *gin.Context) {
	user := runEnv.getUserFromCookie(c)

	c.String(http.StatusOK, user.Login)
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

func (runEnv Env) handlerOrderRegister(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, "user not authenticated")
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.Error().Err(err).Msg("can't read request body")
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	orderID, err := strconv.Atoi(string(body))
	if err != nil {
		logger.Error().Msg("order format is wrong")
		c.String(http.StatusUnprocessableEntity, "order format is wrong")
		return
	}

	order, err := runEnv.orders.Create(c, orderID, int(user.ID))
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, "db error")
		return
	}
	// order will be not nil only if new order was inserted successfully
	if order.Number > 0 {
		c.String(http.StatusAccepted, "")
		return
	}

	// if we here, it probably means such order id was already in db
	order, err = runEnv.orders.GetByNumber(c, orderID)
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, "db error")
		return
	}
	if order.Number == 0 {
		logger.Error().Msgf("cannot neither insert nor find order with id %d", orderID)
		c.String(http.StatusBadRequest, "")
		return
	}

	if order.UserID == user.ID {
		logger.Info().Msgf(
			"order %d already inserted by this user %d",
			order.Number, order.UserID,
		)
		c.String(http.StatusOK, "")
		return
	} else {
		logger.Warn().Msgf(
			"order %d already inserted by another user (%d, not %d)",
			order.Number, order.UserID, user.ID,
		)
		c.String(http.StatusConflict, "")
		return
	}
}

func (runEnv Env) handlerOrdersList(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, "user not authenticated")
		return
	}

	orders, err := runEnv.orders.GetListByUserID(c, int(user.ID))
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, "db error")
		return
	}

	if len(*orders) == 0 {
		c.String(http.StatusNoContent, "")
		return
	}

	c.PureJSON(http.StatusOK, orders)
}

type balanceResponse struct {
	Current   decimal.Decimal `json:"current"`
	Withdrawn decimal.Decimal `json:"withdrawn"`
}

func (runEnv Env) handlerUserGetBalance(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, "user not authenticated")
		return
	}

	decimal.MarshalJSONWithoutQuotes = true
	c.PureJSON(http.StatusOK, balanceResponse{user.Balance, user.Withdrawn})
}
