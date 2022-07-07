package app

import (
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"
	"github.com/skaurus/yandex-practicum-go-exam/internal/ledger"
	"github.com/skaurus/yandex-practicum-go-exam/internal/orders"
	"github.com/skaurus/yandex-practicum-go-exam/internal/transport/http"
	"github.com/skaurus/yandex-practicum-go-exam/internal/users"
)

type Runner interface {
	Start(errCh chan<- error) error
	Stop() error
}

func Run(runEnv *env.Env) (Runner, error) {
	var packageEnvs = env.PackageEnvs{}
	if err := users.InitEnv(packageEnvs, runEnv); err != nil {
		return nil, err
	}
	if err := orders.InitEnv(packageEnvs, runEnv); err != nil {
		return nil, err
	}
	if err := ledger.InitEnv(packageEnvs, runEnv); err != nil {
		return nil, err
	}

	return http.Runner(packageEnvs, runEnv)
}
