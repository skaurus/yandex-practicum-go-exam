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
	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"

	"github.com/rs/zerolog"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func initConfig() (err error) {
	const (
		defaultRunAddress       = "localhost:8080"
		defaultAccrualAddress   = "localhost:7979"
		defaultCookieDomain     = "localhost"
		defaultDBConnectTimeout = 1 * time.Second
		defaultDBQueryTimeout   = 1 * time.Second
	)

	// In production these values should be placed in a config file (I would
	// prefer toml format) that is not committed to the repo, but test environment
	// of this project does not allow me to somehow define a config, so I use
	// default values as kind of config.
	viper.SetDefault("RUN_ADDRESS", defaultRunAddress)
	viper.SetDefault("ACCRUAL_SYSTEM_ADDRESS", defaultAccrualAddress)
	viper.SetDefault("COOKIE_DOMAIN", defaultCookieDomain)
	viper.SetDefault("DB_CONNECT_TIMEOUT", defaultDBConnectTimeout)
	viper.SetDefault("DB_QUERY_TIMEOUT", defaultDBQueryTimeout)
	viper.SetDefault("PASSWORD_SECRET", "forum-prefix-guitar")

	// I chose that key names are the same as ENV variables (why not)
	if err = viper.BindEnv("RUN_ADDRESS", "RUN_ADDRESS"); err != nil {
		return err
	}
	if err = viper.BindEnv("DATABASE_URI", "DATABASE_URI"); err != nil {
		return err
	}
	if err = viper.BindEnv("ACCRUAL_SYSTEM_ADDRESS", "ACCRUAL_SYSTEM_ADDRESS"); err != nil {
		return err
	}

	// In the training project (not this one, this is graduate project) we were
	// required to make ENV priority higher than flags; viper lib holds it other
	// way around (and I prefer it so). Hope it is not a problem for the autotests.
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

func initDB() (db.DB, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		viper.Get("DB_CONNECT_TIMEOUT").(time.Duration),
	)
	defer cancel()
	return db.Connect(ctx)
}

func main() {
	rand.Seed(time.Now().UnixNano())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if err := initConfig(); err != nil {
		panic(err)
	}

	dbInstance, err := initDB()
	if err != nil {
		panic(err)
	}

	logger := initLogging(os.Stdout)

	runEnv := env.Init(dbInstance, &logger)

	router := app.SetupRouter(&runEnv)
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
	// If cancel() fires, Shutdown will be executed forcefully, even if there
	// is still requests processing.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal().Err(err).Msgf("can't shutdown the server because of %v", err)
	}

	logger.Info().Msg("exited")
}
