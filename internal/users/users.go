package users

import (
	"context"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"

	"github.com/rs/zerolog"
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

type User struct {
	ID       uint32
	Login    string
	Password string
}

type Request struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (runEnv Env) Create(ctx context.Context, req Request) (u User, err error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	_, err = runEnv.DB().QueryRow(
		ctx,
		&u,
		"INSERT INTO users (login, password) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING id, login, password",
		req.Login, req.Password,
	)
	// Если была ошибка - она попадёт в return; если был конфликт (такой логин
	// уже есть в базе) - то u будет пустым. То есть никакая обработка не нужна
	return
}

func (runEnv Env) GetByLogin(ctx context.Context, login string) (u User, err error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	_, err = runEnv.DB().QueryRow(
		ctx,
		&u,
		"SELECT id, login, password FROM users WHERE login = $1",
		login,
	)
	// Если была ошибка - она попадёт в return; если был !found (_, пропущенный
	// return из QueryRow) - то u будет пустым. То есть никакая обработка не нужна
	return
}
