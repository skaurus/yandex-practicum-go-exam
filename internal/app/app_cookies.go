package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

// RandStringN - see https://stackoverflow.com/a/31832326/320345
func RandStringN(n int) string {
	const asciiSymbols = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = asciiSymbols[rand.Int63()%int64(len(asciiSymbols))]
	}
	return string(b)
}

const (
	uniqCookieName   = "uniq"
	uniqCookieMaxAge = time.Duration(1e9 * 60 * 60 * 24 * 365) // seconds
	// https://edoceo.com/dev/mnemonic-password-generator
	// It would be better to take this from config, but alas, we don't have one.
	cookieSecretKey = "epoxy-equator-human"
)

func (runEnv Env) getSignedCookie(c *gin.Context, cookieName string) (found bool, decodedValue string) {
	logger := runEnv.Logger()

	// I use for which is executed exactly once as a syntactic sugar. Inside I
	// have 4 successive checks and each next check have meaning only if all
	// previous checks succeeded. I could have wrapped ifs inside each other
	// and that would be ugly and hard to read. This is way prettier.
	for range []int{1} {
		// 1. trying to read uniq cookie
		cookieValue, err := c.Cookie(cookieName)
		if err != nil { // there was no cookie
			logger.Info().Msgf("no %s cookie", cookieName)
			break
		}

		// 2. trying to get cookie value and its signature
		// Unfortunately, Cut appeared only in Go 1.18 and project tests use 1.17
		//maybeUniq, sign, found := strings.Cut(cookieValue, "-")
		parts := strings.SplitN(cookieValue, "-", 2)
		// "maybe" means that we not sure if it is valid value until we check
		// the signature
		maybeValue, sign := parts[0], parts[1]
		if len(sign) == 0 {
			logger.Error().Msgf("%s cookie don't have separator", cookieName)
			break
		}

		// 3. trying to decipher signature of the cookie
		sign1, err := base64.StdEncoding.DecodeString(sign)
		if err != nil {
			logger.Error().Msgf("%s cookie signature can't be decoded", cookieName)
			break
		}

		hmacer := hmac.New(sha256.New, []byte(cookieSecretKey))
		hmacer.Write([]byte(maybeValue))
		sign2 := hmacer.Sum(nil)
		if !hmac.Equal(sign1, sign2) {
			logger.Error().Msgf("%s cookie signature is wrong", cookieName)
			break
		}

		found = true
		decodedValue = maybeValue
	}

	// Method is quite long, so although I could write just "return",
	// I prefer to make this explicit
	return found, decodedValue
}

func (runEnv Env) setSignedCookie(c *gin.Context, cookieName, cookieValue string, maxAge int, path string, secure bool, httpOnly bool) {
	logger := runEnv.Logger()

	hmacer.Reset()
	hmacer.Write([]byte(cookieValue))
	sign := hmacer.Sum(nil)

	cookieValue = fmt.Sprintf("%s-%s", cookieValue, base64.StdEncoding.EncodeToString(sign))

	c.SetCookie(
		cookieName, cookieValue, maxAge, path,
		viper.Get("COOKIE_DOMAIN").(string), secure, httpOnly,
	)
	logger.Info().Msgf("set %s cookie to %s", cookieName, cookieValue)
}

// middlewareSetCookies - write/read transient cookies
func (runEnv Env) middlewareSetCookies(c *gin.Context) {
	found, uniq := runEnv.getSignedCookie(c, uniqCookieName)
	if !found || len(uniq) == 0 {
		uniq = RandStringN(8)
		runEnv.setSignedCookie(
			c,
			uniqCookieName, uniq,
			int(uniqCookieMaxAge.Seconds()), "/", false, true,
		)
	}

	c.Set("uniq", uniq)

	c.Next()
}
