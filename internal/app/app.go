package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"hash"
	"io"
	"os"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"
	"github.com/skaurus/yandex-practicum-go-exam/internal/orders"
	"github.com/skaurus/yandex-practicum-go-exam/internal/users"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type Env struct {
	Env    *env.Env
	users  users.Env
	orders orders.Env
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

	return router
}
