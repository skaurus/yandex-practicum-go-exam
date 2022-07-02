package app

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/ledger"
	"github.com/skaurus/yandex-practicum-go-exam/internal/orders"
	"github.com/skaurus/yandex-practicum-go-exam/internal/users"

	"github.com/gin-gonic/gin"
	jsoniter "github.com/json-iterator/go"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
	"github.com/theplant/luhn"
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

// I intentionally use [0-9] as opposed to \d. Surprisingly, in some regexp
// implementations \d also matches utf-8 "digit" glyphs like this one:
// https://www.fileformat.info/info/unicode/char/07c1/index.htm
// This does not seem to be the case with Go though... but anyway.
var onlyDigitsRe = regexp.MustCompile(`^[0-9]+$`)

func (runEnv Env) handlerOrderRegister(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, "user not authenticated")
		return
	}

	orderNumber, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.Error().Err(err).Msg("can't read request body")
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	if !onlyDigitsRe.Match(orderNumber) {
		logger.Error().Msg("order format is wrong")
		c.String(http.StatusUnprocessableEntity, "order format is wrong")
		return
	}
	intNumber, err := strconv.Atoi(string(orderNumber))
	if err != nil || !luhn.Valid(intNumber) {
		logger.Error().Msg("order format is wrong")
		c.String(http.StatusUnprocessableEntity, "order format is wrong")
		return
	}

	order, err := runEnv.orders.Create(c, string(orderNumber), int(user.ID))
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, "db error")
		return
	}
	// order will be not nil only if new order was inserted successfully
	if len(order.Number) > 0 {
		c.String(http.StatusAccepted, "")
		return
	}

	// if we here, it probably means such order id was already in db
	order, err = runEnv.orders.GetByNumber(c, string(orderNumber))
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusBadRequest, "db error")
		return
	}
	if len(order.Number) == 0 {
		logger.Error().Msgf("cannot neither insert nor find order number %s", order.Number)
		c.String(http.StatusBadRequest, "")
		return
	}

	if order.UserID == user.ID {
		logger.Info().Msgf(
			"order %s already inserted by this user %d",
			order.Number, order.UserID,
		)
		c.String(http.StatusOK, "")
		return
	} else {
		logger.Warn().Msgf(
			"order %s already inserted by another user (%d, not %d)",
			order.Number, order.UserID, user.ID,
		)
		c.String(http.StatusConflict, "")
		return
	}
}

type accrualResponse struct {
	OrderNumber string           `json:"order"`
	Status      orders.Status    `json:"status"`
	Accrual     *decimal.Decimal `json:"accrual,omitempty"`
}

// processOrders will search for orders not in a final status, check each one
// against accrual service, and sleep for 1 second between runs (unless told
// to wait longer).
func (runEnv Env) processOrders() {
	logger := runEnv.Logger()

	accrualURLPrefix := viper.Get("ACCRUAL_SYSTEM_ADDRESS").(string) + "/api/orders/"
	for {
		// On one hand, it would be more pretty to sleep AFTER we have done some
		// work; but that means that Sleep has to be also added before EACH
		// continue; and this is not pretty, not convenient, and easy to forget
		time.Sleep(time.Second)

		orderList, err := runEnv.orders.GetList(
			context.Background(),
			"status != ANY($1)",
			"",
			[]string{string(orders.StatusInvalid), string(orders.StatusProcessed)},
		)
		if err != nil {
			logger.Error().Err(err).Msg("db error")
			continue
		}

	toNextOrder:
		for _, order := range *orderList {
			time.Sleep(time.Second)

			res, err := http.Get(accrualURLPrefix + order.Number)
			if err != nil {
				logger.Error().Err(err).Msg("http get error")
				continue toNextOrder
			}

			resStatusCode := res.StatusCode

			body, err := io.ReadAll(res.Body)
			if err != nil {
				logger.Error().Err(err).Msg("can't read response body")
				res.Body.Close()
				continue toNextOrder
			}

			err = res.Body.Close()
			if err != nil {
				logger.Error().Err(err).Msg("can't close response body")
				continue toNextOrder
			}

			switch resStatusCode {
			case http.StatusOK:
			case http.StatusTooManyRequests:
				v := res.Header.Get("Retry-After")
				seconds, err := strconv.Atoi(v)
				if err != nil {
					logger.Error().Err(err).Msgf("wrong value of Retry-After header: %s", v)
					continue toNextOrder
				}
				time.Sleep(time.Duration(seconds) * time.Second)
				continue toNextOrder
			default:
				continue toNextOrder
			}

			var data accrualResponse
			err = json.Unmarshal(body, &data)
			if err != nil {
				logger.Error().Err(err).Msg("can't parse body")
				continue toNextOrder
			}
			if data.OrderNumber != order.Number {
				logger.Error().Msgf(
					"response contains different to requested order number: %s, %s",
					data.OrderNumber, order.Number,
				)
				continue toNextOrder
			}

			// order is not yet updated
			if data.Status == order.Status {
				continue toNextOrder
			}

			order.Status = data.Status
			order.Accrual = data.Accrual
			err = runEnv.orders.Accrue(context.Background(), runEnv.ledger, order)
			switch err {
			case nil:
				logger.Info().Msgf("order %s is updated to %v", order.Number, order)
			case orders.ErrNoSuchUser: // should never happen
				logger.Error().Err(err).Msg("can't find order's user")
				continue toNextOrder
			default:
				logger.Error().Err(err).Msg("db error")
				continue toNextOrder
			}
		}
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
		c.String(http.StatusInternalServerError, "db error")
		return
	}

	if len(*orders) == 0 {
		c.String(http.StatusNoContent, "")
		return
	}

	decimal.MarshalJSONWithoutQuotes = true
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

type withdrawRequest struct {
	OrderNumber string          `json:"order"`
	Sum         decimal.Decimal `json:"sum"`
}

func (runEnv Env) handlerUserWithdraw(c *gin.Context) {
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

	var data withdrawRequest
	err = json.Unmarshal(body, &data)
	if err != nil {
		logger.Error().Err(err).Msg("can't parse body")
		c.String(http.StatusBadRequest, "can't parse json")
		return
	}

	// I check more errors than required, but this seems like a good idea
	if !onlyDigitsRe.Match([]byte(data.OrderNumber)) || !data.Sum.IsPositive() {
		logger.Error().Msg("request is wrong")
		c.String(http.StatusBadRequest, "request is wrong")
		return
	}

	err = runEnv.users.Withdraw(c, runEnv.ledger, int(user.ID), data.OrderNumber, &data.Sum)
	switch err {
	case nil:
		c.String(http.StatusOK, "")
	case users.ErrInsufficientFunds:
		c.String(http.StatusPaymentRequired, err.Error())
	case users.ErrNoSuchUser: // should never happen
		c.String(http.StatusBadRequest, err.Error())
	default:
		c.String(http.StatusInternalServerError, err.Error())
	}
}

func (runEnv Env) handlerUserWithdrawalsList(c *gin.Context) {
	logger := runEnv.Logger()

	user := runEnv.getUserFromCookie(c)
	if user == nil {
		logger.Info().Msg("user not authenticated")
		c.String(http.StatusUnauthorized, "user not authenticated")
		return
	}

	credits, err := runEnv.ledger.GetList(
		c,
		"user_id = $1 AND operation = $2",
		"ORDER BY processed_at ASC",
		int(user.ID), ledger.TransactionCredit,
	)
	if err != nil {
		logger.Error().Err(err).Msgf("db error: %v", err)
		c.String(http.StatusInternalServerError, "db error")
		return
	}

	if len(*credits) == 0 {
		c.String(http.StatusNoContent, "")
		return
	}

	c.PureJSON(http.StatusOK, credits)
}
