package env

import (
	"errors"
	"fmt"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"

	"github.com/rs/zerolog"
)

type Env struct {
	db     db.DB
	logger *zerolog.Logger
}

type Interface interface {
	DB() db.DB
	Logger() *zerolog.Logger
}

type EnvsStorage map[string]Interface

func (env Env) DB() db.DB {
	return env.db
}

func (env Env) Logger() *zerolog.Logger {
	return env.logger
}

func Init(dbInstance db.DB, loggerInstance *zerolog.Logger) Env {
	return Env{dbInstance, loggerInstance}
}

var PackageEnvs = EnvsStorage{}

var ErrDuplicateModelName = errors.New("duplicate model name")

func InitModelEnv(modelName string, modelEnv Interface) error {
	if _, ok := PackageEnvs[modelName]; ok {
		return fmt.Errorf("%w with model %s", ErrDuplicateModelName, modelName)
	}
	PackageEnvs[modelName] = modelEnv
	return nil
}
