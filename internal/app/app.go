package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

type Env struct {
	Env *env.Env
}

func (runEnv Env) DB() db.DB {
	return runEnv.Env.DB()
}

func (runEnv Env) Logger() *zerolog.Logger {
	return runEnv.Env.Logger()
}

func SetupRouter(env *env.Env) *gin.Engine {
	gin.DisableConsoleColor()
	gin.DefaultWriter = io.MultiWriter(os.Stdout)

	runEnv := Env{
		Env: env,
	}

	router := gin.Default()
	router.Use(runEnv.middlewareGzipCompression)
	router.Use(runEnv.middlewareSetCookies)

	router.POST("/api/user/register", runEnv.handlerUserRegister)
	router.GET("/ping", runEnv.handlerPing)

	return router
}

const (
	uniqCookieName   = "uniq"
	uniqCookieMaxAge = time.Duration(1e9 * 60 * 60 * 24 * 365) // seconds
	// https://edoceo.com/dev/mnemonic-password-generator
	// лучше бы, конечно, брать из конфига, а не коммитить в код; но конфигурация
	// приложения задана условиями задания
	cookieSecretKey = "epoxy-equator-human"
	asciiSymbols    = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// RandStringN - see https://stackoverflow.com/a/31832326/320345
func RandStringN(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = asciiSymbols[rand.Int63()%int64(len(asciiSymbols))]
	}
	return string(b)
}

var hmacer hash.Hash

// middlewareSetCookies - проставляем/читаем куки
func (runEnv Env) middlewareSetCookies(c *gin.Context) {
	logger := runEnv.Logger()

	var uniq string
	// блок с несколькими последовательными проверками - это способ не делать
	// вложенные один в другой if (success) { ... }
	// range написан так, чтобы for был выполнен ровно один раз
	for range []int{1} {
		// 1. пытаемся прочитать куку уника
		cookieValue, err := c.Cookie(uniqCookieName)
		if err != nil { // куки не было
			logger.Info().Msg("no uniq cookie")
			break
		}

		// 2. пытаемся достать из куки айди и подпись
		// Cut появился только в go 1.18 ((
		//maybeUniq, sign, found := strings.Cut(cookieValue, "-")
		parts := strings.SplitN(cookieValue, "-", 2)
		maybeUniq, sign := parts[0], parts[1]
		if len(sign) == 0 {
			logger.Error().Msg("uniq cookie don't have separator")
			break
		}

		// 3. пытаемся расшифровать подпись куки уника
		sign1, err := hex.DecodeString(sign)
		if err != nil {
			logger.Error().Msg("uniq cookie signature can't be decoded")
			break
		}

		hmacer := hmac.New(sha256.New, []byte(cookieSecretKey))
		hmacer.Write([]byte(maybeUniq))
		sign2 := hmacer.Sum(nil)
		if !hmac.Equal(sign1, sign2) {
			logger.Error().Msg("uniq cookie signature is wrong")
			break
		}

		uniq = maybeUniq
	}

	if len(uniq) == 0 {
		uniq = RandStringN(8)
		if hmacer == nil {
			hmacer = hmac.New(sha256.New, []byte(cookieSecretKey))
		}
		hmacer.Reset()
		hmacer.Write([]byte(uniq))
		sign := hmacer.Sum(nil)
		cookieValue := fmt.Sprintf("%s-%s", uniq, hex.EncodeToString(sign))
		c.SetCookie(
			uniqCookieName, cookieValue, int(uniqCookieMaxAge.Seconds()), "/",
			viper.Get("COOKIE_DOMAIN").(string), false, true,
		)
		logger.Info().Msg("set uniq cookie " + cookieValue)
	}

	c.Set("uniq", uniq)

	c.Next()
}
