package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/logger"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"
	"github.com/skaurus/yandex-practicum-go-exam/internal/ledger"
	"github.com/skaurus/yandex-practicum-go-exam/internal/orders"
	"github.com/skaurus/yandex-practicum-go-exam/internal/users"
)

const modelName = "http"

type Env struct {
	env *env.Env
}

func (runEnv Env) DB() db.DB {
	return runEnv.env.DB()
}

func (runEnv Env) Logger() *zerolog.Logger {
	return runEnv.env.Logger()
}

var usersEnv users.Env
var ordersEnv orders.Env
var ledgerEnv ledger.Env

type runner struct {
	gin *gin.Engine
	srv *http.Server
}

func Runner(packageEnvs env.PackageEnvs, runEnv *env.Env) (*runner, error) {
	err := env.InitModelEnv(packageEnvs, modelName, Env{env: runEnv})
	if err != nil {
		return &runner{}, err
	}

	usersEnv = packageEnvs[users.ModelName].(users.Env)
	ordersEnv = packageEnvs[orders.ModelName].(orders.Env)
	ledgerEnv = packageEnvs[ledger.ModelName].(ledger.Env)
	localEnv := packageEnvs[modelName].(Env)

	hmacer = hmac.New(sha256.New, []byte(cookieSecretKey))

	gin.DisableConsoleColor()
	gin.DefaultWriter = io.MultiWriter(os.Stdout)

	router := gin.Default()
	router.Use(logger.SetLogger())
	router.Use(localEnv.middlewareGzipCompression)
	router.Use(localEnv.middlewareSetCookies)

	router.GET("/saymyname", localEnv.handlerSayMyName)

	router.POST("/api/user/register", localEnv.handlerUserRegister)
	router.POST("/api/user/login", localEnv.handlerUserLogin)
	router.POST("/api/user/orders", localEnv.handlerOrderRegister)
	go localEnv.processOrders()
	router.GET("/api/user/orders", localEnv.handlerOrdersList)
	router.GET("/api/user/balance", localEnv.handlerUserGetBalance)
	router.POST("/api/user/balance/withdraw", localEnv.handlerUserWithdraw)
	router.GET("/api/user/balance/withdrawals", localEnv.handlerUserWithdrawalsList)

	return &runner{gin: router, srv: nil}, nil
}

func (runner *runner) Start(errCh chan<- error) error {
	runner.srv = &http.Server{
		Addr:    viper.Get("RUN_ADDRESS").(string),
		Handler: runner.gin,
	}
	go func() {
		err := runner.srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- errors.New("server can't listen")
		}
	}()
	return nil
}

func (runner *runner) Stop() error {
	// If cancel() fires, Shutdown will be executed forcefully, even if there
	// are still requests processing.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runner.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("can't shutdown the server because of %w", err)
	}
	return nil
}
