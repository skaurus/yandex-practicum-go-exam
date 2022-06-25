package main

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/app"

	"github.com/rs/zerolog"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func initConfig() (err error) {
	const (
		defaultRunAddress     = "localhost:8080"
		defaultAccrualAddress = "localhost:7979"
		defaultCookieDomain   = "localhost"
	)

	viper.SetDefault("RUN_ADDRESS", defaultRunAddress)
	viper.SetDefault("ACCRUAL_SYSTEM_ADDRESS", defaultAccrualAddress)
	viper.SetDefault("COOKIE_DOMAIN", defaultCookieDomain)

	// выбранные имена ключей конфига совпадают с env-переменными (why not)
	if err = viper.BindEnv("RUN_ADDRESS", "RUN_ADDRESS"); err != nil {
		return err
	}
	if err = viper.BindEnv("DATABASE_URI", "DATABASE_URI"); err != nil {
		return err
	}
	if err = viper.BindEnv("ACCRUAL_SYSTEM_ADDRESS", "ACCRUAL_SYSTEM_ADDRESS"); err != nil {
		return err
	}

	// есть нюанс. в прохождении курса требовалось, чтобы ENV имело приоритет
	// перед флагами; а у viper приоритет обратный (и мне кажется, что так лучше)
	pflag.String("a", defaultRunAddress, "run address of the app")
	if err = viper.BindPFlag("RUN_ADDRESS", pflag.Lookup("a")); err != nil {
		return err
	}
	pflag.String("d", "", "database connect string")
	if err = viper.BindPFlag("DATABASE_URI", pflag.Lookup("d")); err != nil {
		return err
	}
	pflag.String("r", defaultAccrualAddress, "network address of accrual system")
	if err = viper.BindPFlag("ACCRUAL_SYSTEM_ADDRESS", pflag.Lookup("r")); err != nil {
		return err
	}

	return nil
}

func initLogging(w io.Writer) zerolog.Logger {
	zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	return zerolog.New(w).With().Timestamp().Logger()
}

func main() {
	rand.Seed(time.Now().UnixNano())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if err := initConfig(); err != nil {
		panic(err)
	}

	logger := initLogging(os.Stdout)

	router := app.SetupRouter(&logger)
	srv := &http.Server{
		Addr:    viper.Get("RUN_ADDRESS").(string),
		Handler: router,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("can't start the server")
		}
	}()

	sig := <-sigCh
	logger.Info().Msgf("got signal %s, exiting\n", sig)
	close(sigCh)
	// когда сработает cancel - Shutdown выполнится принудительно, даже если
	// сервер ещё не дождался завершения всех запросов
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msgf("can't shutdown the server because of %v", err)
	}

	logger.Info().Msg("exited")
}
