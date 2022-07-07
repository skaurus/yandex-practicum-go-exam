package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"
	"github.com/skaurus/yandex-practicum-go-exam/internal/ledger"
	"github.com/skaurus/yandex-practicum-go-exam/internal/orders"
	"github.com/skaurus/yandex-practicum-go-exam/internal/users"

	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
	"github.com/theplant/luhn"
)

const ModelName = "controller"

type Env struct {
	env *env.Env
}

func (runEnv Env) DB() db.DB {
	return runEnv.env.DB()
}

func (runEnv Env) Logger() *zerolog.Logger {
	return runEnv.env.Logger()
}

func InitEnv(runEnv *env.Env) error {
	return env.InitModelEnv(ModelName, Env{env: runEnv})
}

func GetEnv() Env {
	runEnv, ok := env.PackageEnvs[ModelName]
	if !ok {
		panic(ModelName + " Env is not yet initialized")
	}
	return runEnv.(Env)
}

var ErrDB = errors.New("db error")
var ErrLoginTaken = errors.New("login already taken")
var ErrWrongAuth = errors.New("wrong login or password")
var ErrUserNotAuthenticated = errors.New("user not authenticated")
var ErrCantParseJSON = errors.New("can't parse json")
var ErrOrderFormatIsWrong = errors.New("order format is wrong")
var ErrOrderCreateFailure = errors.New("cannot neither insert nor find order number %s")
var ErrOrderAlreadyExists = errors.New("order already inserted by current user")
var ErrOrderRegisteredByAnotherUser = errors.New("order already inserted by another user")
var ErrRequestIsWrong = errors.New("request is wrong")

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func (runEnv Env) CreateUser(ctx context.Context, create users.Auth) (user *users.User, err error) {
	user, err = users.GetEnv().Create(ctx, create)
	if err != nil {
		return
	}
	if user.ID == 0 {
		// Not 100% sure that this is the only reason to have an empty user.
		// But it is for sure the most probable one.
		err = ErrLoginTaken
		return
	}

	return
}

func (runEnv Env) AuthUser(ctx context.Context, auth users.Auth) (user *users.User, err error) {
	user, err = users.GetEnv().GetByLogin(ctx, auth.Login)
	if err != nil {
		return
	}
	if user.ID == 0 {
		err = ErrWrongAuth
		return
	}
	if ok := users.GetEnv().CheckPassword(user, auth.Password); !ok {
		err = ErrWrongAuth
		return
	}

	return
}

// I intentionally use [0-9] as opposed to \d. Surprisingly, in some regexp
// implementations \d also matches utf-8 "digit" glyphs like this one:
// https://www.fileformat.info/info/unicode/char/07c1/index.htm
// This does not seem to be the case with Go though... but anyway.
var onlyDigitsRe = regexp.MustCompile(`^[0-9]+$`)

func (runEnv Env) RegisterOrder(ctx context.Context, create orders.Create) (order *orders.Order, err error) {
	if !onlyDigitsRe.Match([]byte(create.Number)) {
		err = ErrOrderFormatIsWrong
		return
	}
	intNumber, err := strconv.Atoi(create.Number)
	if err != nil || !luhn.Valid(intNumber) {
		err = ErrOrderFormatIsWrong
		return
	}

	order, err = orders.GetEnv().Create(ctx, create.Number, int(create.UserID))
	if err != nil {
		return
	}
	// order will be not nil only if new order was inserted successfully
	if len(order.Number) > 0 {
		return
	}

	// if we here, it probably means such order id was already in db
	order, err = orders.GetEnv().GetByNumber(ctx, create.Number)
	if err != nil {
		return
	}
	if len(order.Number) == 0 {
		err = fmt.Errorf("can't create order %s because of %w", order.Number, ErrOrderCreateFailure)
		return
	}

	if order.UserID == create.UserID {
		err = ErrOrderAlreadyExists
		return
	} else {
		err = ErrOrderRegisteredByAnotherUser
		return
	}
}

type accrualResponse struct {
	OrderNumber string           `json:"order"`
	Status      orders.Status    `json:"status"`
	Accrual     *decimal.Decimal `json:"accrual,omitempty"`
}

// ProcessOrders will search for orders not in a final status, check each one
// against accrual service, and sleep for 1 second between runs (unless told
// to wait longer).
func (runEnv Env) ProcessOrders() {
	logger := runEnv.Logger()

	accrualURLPrefix := viper.Get("ACCRUAL_SYSTEM_ADDRESS").(string) + "/api/orders/"
	for {
		// On one hand, it would be more pretty to sleep AFTER we have done some
		// work; but that means that Sleep has to be also added before EACH
		// continue; and this is not pretty, not convenient, and easy to forget
		time.Sleep(time.Second)

		orderList, err := orders.GetEnv().GetList(
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
				err = res.Body.Close()
				if err != nil {
					logger.Error().Err(err).Msg("can't close response body")
				}
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
			err = orders.GetEnv().Accrue(context.Background(), &order)
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

func (runEnv Env) ListOrders(ctx context.Context, userID int) (orderList *[]orders.Order, err error) {
	return orders.GetEnv().GetListByUserID(ctx, userID)
}

func (runEnv Env) WithdrawFromUser(ctx context.Context, userID int, orderNumber string, sum *decimal.Decimal) (err error) {
	// I check more errors than required, but this seems like a good idea
	if !onlyDigitsRe.Match([]byte(orderNumber)) || !sum.IsPositive() {
		err = ErrRequestIsWrong
		return
	}
	return users.GetEnv().Withdraw(ctx, userID, orderNumber, sum)
}

func (runEnv Env) ListUserWithdrawals(ctx context.Context, userID int) (ts *[]ledger.Transaction, err error) {
	return ledger.GetEnv().GetList(
		ctx,
		"user_id = $1 AND operation = $2",
		"ORDER BY processed_at ASC",
		userID, ledger.TransactionCredit,
	)
}
