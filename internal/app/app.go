package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"hash"
	"io"
	"os"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"
	"github.com/skaurus/yandex-practicum-go-exam/internal/ledger"
	"github.com/skaurus/yandex-practicum-go-exam/internal/orders"
	"github.com/skaurus/yandex-practicum-go-exam/internal/users"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type Env struct {
	Env    *env.Env
	users  users.Env
	orders orders.Env
	ledger ledger.Env
}

func (runEnv Env) DB() db.DB {
	return runEnv.Env.DB()
}

func (runEnv Env) Logger() *zerolog.Logger {
	return runEnv.Env.Logger()
}

var hmacer hash.Hash

func SetupRouter(env *env.Env) *gin.Engine {
	gin.DisableConsoleColor()
	gin.DefaultWriter = io.MultiWriter(os.Stdout)

	runEnv := Env{
		Env: env,
		// Jumping through hoops so:
		// a) every package will use the same env;
		// b) we could use env as a method receiver in every package, which
		//    would be convenient. Method receiver must be of type from the
		//    same package as method itself.
		users:  users.Env{Env: env},
		orders: orders.Env{Env: env},
		ledger: ledger.Env{Env: env},
	}

	hmacer = hmac.New(sha256.New, []byte(cookieSecretKey))

	router := gin.Default()
	router.Use(runEnv.middlewareGzipCompression)
	router.Use(runEnv.middlewareSetCookies)

	router.GET("/saymyname", runEnv.handlerSayMyName)

	router.POST("/api/user/register", runEnv.handlerUserRegister)
	router.POST("/api/user/login", runEnv.handlerUserLogin)
	router.POST("/api/user/orders", runEnv.handlerOrderRegister)
	router.GET("/api/user/orders", runEnv.handlerOrdersList)
	router.GET("/api/user/balance", runEnv.handlerUserGetBalance)
	router.POST("/api/user/balance/withdraw", runEnv.handlerUserWithdraw)
	router.GET("/api/user/balance/withdrawals", runEnv.handlerUserWithdrawalsList)

	return router
}
