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

type PackageEnvs map[string]Interface

func (env Env) DB() db.DB {
	return env.db
}

func (env Env) Logger() *zerolog.Logger {
	return env.logger
}

func Init(dbInstance db.DB, loggerInstance *zerolog.Logger) Env {
	return Env{dbInstance, loggerInstance}
}

var ErrDuplicateModelName = errors.New("duplicate model name")

func InitModelEnv(packageEnvs PackageEnvs, modelName string, modelEnv Interface) error {
	if _, ok := packageEnvs[modelName]; ok {
		return fmt.Errorf("%w with model %s", ErrDuplicateModelName, modelName)
	}
	packageEnvs[modelName] = modelEnv
	return nil
}
