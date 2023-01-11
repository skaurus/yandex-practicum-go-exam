package http

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/controllers"
	"github.com/skaurus/yandex-practicum-go-exam/internal/orders"
	"github.com/skaurus/yandex-practicum-go-exam/internal/users"

	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
	"github.com/shopspring/decimal"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func (runEnv localEnv) parseUserRequest(c *gin.Context) (ok bool, req users.Auth) {
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
		c.String(http.StatusBadRequest, controllers.ErrCantParseJSON.Error())
		return
	}
	if len(req.Login) == 0 || len(req.Password) == 0 {
		logger.Warn().Msg("empty login or password")
		c.String(http.StatusBadRequest, controllers.ErrWrongAuth.Error())
		return
	}

	return true, req
}

const (
	loginCookieName   = "whoami"
	loginCookieMaxAge = time.Duration(1e9 * 60 * 60 * 24 * 365) // seconds
)

func (runEnv localEnv) setAuthCookie(c *gin.Context, u *users.User) {
	loginBytes := []byte(u.Login)

	runEnv.setSignedCookie(
		c,
		loginCookieName, base64.StdEncoding.EncodeToString(loginBytes),
		int(loginCookieMaxAge.Seconds()), "/", false, true,
	)
}

func (runEnv localEnv) getUserFromCookie(c *gin.Context) (u *users.User) {
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

	u, err = users.GetEnv().GetByLogin(c, string(loginBytes))
	if err != nil {
		logger.Error().Err(err).Msg("db error")
	}

	return
}

func (runEnv localEnv) handlerSayMyName(c *gin.Context) {
	user := runEnv.getUserFromCookie(c)

	c.String(http.StatusOK, user.Login)
}

func (runEnv localEnv) handlerUserRegister(c *gin.Context) {
	logger := runEnv.Logger()

	ok, req := runEnv.parseUserRequest(c)
	if !ok {
		return
	}

	user, err := controllers.GetEnv().CreateUser(c, req)
	if errors.Is(err, controllers.ErrLoginTaken) {
		c.String(http.StatusConflict, controllers.ErrLoginTaken.Error())
		return
	} else if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, controllers.ErrDB.Error())
		return
	}

	runEnv.setAuthCookie(c, user)

	c.String(http.StatusOK, "")
}

func (runEnv localEnv) handlerUserLogin(c *gin.Context) {
	logger := runEnv.Logger()

	ok, req := runEnv.parseUserRequest(c)
	if !ok {
		return
	}

	user, err := controllers.GetEnv().AuthUser(c, req)
	if errors.Is(err, controllers.ErrWrongAuth) {
		c.String(http.StatusConflict, controllers.ErrWrongAuth.Error())
		return
	} else if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, controllers.ErrDB.Error())
		return
	}

	runEnv.setAuthCookie(c, user)

	c.String(http.StatusOK, "")
}

func (runEnv localEnv) handlerOrderRegister(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, controllers.ErrUserNotAuthenticated.Error())
		return
	}

	orderNumber, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.Error().Err(err).Msg("can't read request body")
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	create := orders.Create{Number: string(orderNumber), UserID: user.ID}
	order, err := controllers.GetEnv().RegisterOrder(c, create)
	switch {
	case err == nil:
		c.String(http.StatusAccepted, "")
	case errors.Is(err, controllers.ErrOrderAlreadyExists):
		logger.Info().Err(err).Msgf(
			"order %s already inserted by this user %d",
			order.Number, order.UserID,
		)
		c.String(http.StatusOK, "")
	case errors.Is(err, controllers.ErrOrderFormatIsWrong):
		logger.Error().Err(err).Msg(controllers.ErrOrderFormatIsWrong.Error())
		c.String(http.StatusUnprocessableEntity, controllers.ErrOrderFormatIsWrong.Error())
	case errors.Is(err, controllers.ErrOrderCreateFailure):
		logger.Error().Err(err).Msgf("cannot neither insert nor find order number %s", order.Number)
		c.String(http.StatusBadRequest, controllers.ErrOrderCreateFailure.Error())
	case errors.Is(err, controllers.ErrOrderRegisteredByAnotherUser):
		logger.Warn().Msgf(
			"order %s already inserted by another user (%d, not %d)",
			order.Number, order.UserID, user.ID,
		)
		c.String(http.StatusConflict, "")
	default:
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, controllers.ErrDB.Error())
	}
}

func (runEnv localEnv) handlerOrdersList(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, controllers.ErrUserNotAuthenticated.Error())
		return
	}

	orderList, err := controllers.GetEnv().ListOrders(c, int(user.ID))
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusInternalServerError, controllers.ErrDB.Error())
		return
	}

	if len(*orderList) == 0 {
		c.String(http.StatusNoContent, "")
		return
	}

	decimal.MarshalJSONWithoutQuotes = true
	c.PureJSON(http.StatusOK, orderList)
}

type balanceResponse struct {
	Current   decimal.Decimal `json:"current"`
	Withdrawn decimal.Decimal `json:"withdrawn"`
}

func (runEnv localEnv) handlerUserGetBalance(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, controllers.ErrUserNotAuthenticated.Error())
		return
	}

	decimal.MarshalJSONWithoutQuotes = true
	c.PureJSON(http.StatusOK, balanceResponse{user.Balance, user.Withdrawn})
}

type withdrawRequest struct {
	OrderNumber string          `json:"order"`
	Sum         decimal.Decimal `json:"sum"`
}

func (runEnv localEnv) handlerUserWithdraw(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, controllers.ErrUserNotAuthenticated.Error())
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.Error().Err(err).Msg("can't read request body")
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	var data withdrawRequest
	err = json.Unmarshal(body, &data)
	if err != nil {
		logger.Error().Err(err).Msg("can't parse body")
		c.String(http.StatusBadRequest, controllers.ErrCantParseJSON.Error())
		return
	}

	err = controllers.GetEnv().WithdrawFromUser(c, int(user.ID), data.OrderNumber, &data.Sum)
	switch {
	case err == nil:
		c.String(http.StatusOK, "")
	case errors.Is(err, controllers.ErrRequestIsWrong):
		logger.Error().Err(err).Msg(controllers.ErrRequestIsWrong.Error())
		c.String(http.StatusBadRequest, controllers.ErrRequestIsWrong.Error())
	case errors.Is(err, users.ErrInsufficientFunds):
		c.String(http.StatusPaymentRequired, err.Error())
	case errors.Is(err, users.ErrNoSuchUser): // should never happen
		c.String(http.StatusBadRequest, err.Error())
	default:
		c.String(http.StatusInternalServerError, err.Error())
	}
}

func (runEnv localEnv) handlerUserWithdrawalsList(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, controllers.ErrUserNotAuthenticated.Error())
		return
	}

	credits, err := controllers.GetEnv().ListUserWithdrawals(c, int(user.ID))
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusInternalServerError, controllers.ErrDB.Error())
		return
	}

	if len(*credits) == 0 {
		c.String(http.StatusNoContent, "")
		return
	}

	c.PureJSON(http.StatusOK, credits)
}
