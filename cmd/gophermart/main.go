package main

import (
	"context"
	"io"
	"math/rand"
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
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("/etc/gophermart/")
	// Config should never be checked in repo, because it usually contains
	// sensitive data like DB credentials. But in this project I have control
	// only over repository.
	viper.AddConfigPath(".")
	err = viper.ReadInConfig()
	if err != nil {
		return err
	}

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
	pflag.String("a", viper.Get("RUN_ADDRESS").(string), "run address of the app")
	if err = viper.BindPFlag("RUN_ADDRESS", pflag.Lookup("a")); err != nil {
		return err
	}
	pflag.String("d", "", "database connect string")
	if err = viper.BindPFlag("DATABASE_URI", pflag.Lookup("d")); err != nil {
		return err
	}
	pflag.String("r", viper.Get("ACCRUAL_SYSTEM_ADDRESS").(string), "network address of accrual system")
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
	connectTimeout, err := time.ParseDuration(viper.Get("DB_CONNECT_TIMEOUT").(string))
	if err != nil {
		return nil, err
	}

	db.QueryTimeout, err = time.ParseDuration(viper.Get("DB_QUERY_TIMEOUT").(string))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	dbInstance, err := db.Connect(ctx)
	cancel()
	if err != nil {
		return nil, err
	}

	err = dbInstance.InitSchema(context.Background())
	if err != nil {
		return nil, err
	}

	return dbInstance, nil
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

	runner, err := app.Run(&runEnv)
	if err != nil {
		panic(err)
	}

	errCh := make(chan error, 10)
	err = runner.Start(errCh)
	if err != nil {
		panic(err)
	}

forLoop:
	for {
		select {
		case err = <-errCh:
			logger.Fatal().Err(err).Msg("error running server")
		case sig := <-sigCh:
			logger.Info().Msgf("got signal %s, exiting\n", sig)
			if err = runner.Stop(); err != nil {
				logger.Fatal().Err(err).Msg("can't shutdown the server")
			}
			break forLoop
		}
	}
	close(sigCh)
	close(errCh)

	logger.Info().Msg("exited")
}
