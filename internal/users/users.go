package users

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"

	"github.com/rs/zerolog"
	"github.com/spf13/viper"
	"golang.org/x/crypto/argon2"
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
		req.Login, HashPassword(req.Password),
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

func HashPassword(password string) string {
	// Использованы весьма мягкие настройки Argon2id, чтобы сберечь ресурсы
	// тестового контейнера. На проде стоило бы увеличить memory до 64Мб.
	hashedBytes := argon2.IDKey(
		[]byte(password),
		[]byte(viper.Get("PASSWORD_SECRET").(string)),
		1,
		16*1024, // 16Мб
		2,
		32,
	)
	// А 1: - для поддержки нескольких схем хеширования паролей, на случай
	// если потом мы решим это хеширование поменять.
	return "1:" + base64.StdEncoding.EncodeToString(hashedBytes)
}

func (runEnv Env) CheckPassword(u User, password string) bool {
	return u.Password == HashPassword(password)
}
