package env

import (
	"github.com/skaurus/yandex-practicum-go-exam/internal/db"

	"github.com/rs/zerolog"
)

type Env struct {
	db     db.DB
	logger *zerolog.Logger
}

func Init(dbInstance db.DB, loggerInstance *zerolog.Logger) Env {
	return Env{dbInstance, loggerInstance}
}

func (runEnv Env) DB() db.DB {
	return runEnv.db
}

func (runEnv Env) Logger() *zerolog.Logger {
	return runEnv.logger
}
