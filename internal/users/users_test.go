package users

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"
)

var userLogin = ""
var userPassword = "password"

func getTestEnv(t *testing.T) Env {
	err := viper.BindEnv("DATABASE_URI", "DATABASE_URI")
	assert.Nilf(t, err, "viper.BindEnv has failed: %v", err)
	viper.SetDefault("DB_QUERY_TIMEOUT", 1*time.Second)
	viper.SetDefault("PASSWORD_SECRET", "secret")

	db, err := db.Connect(context.Background())
	assert.Nilf(t, err, "connect has failed with error %v", err)
	assert.NotNil(t, db)
	assert.NotNil(t, db.Handle())
	assert.IsType(t, &pgxpool.Pool{}, db.Handle())

	_, err = db.Exec(context.Background(), "DELETE FROM users WHERE login = $1 OR password = $2", userLogin, userPassword)
	assert.Nilf(t, err, "cleaning up db has failed with error %v", err)

	zlog := zerolog.New(os.Stdout)
	env := env.Init(db, &zlog)
	return Env{&env}
}

func Test_CreateAndGet(t *testing.T) {
	runEnv := getTestEnv(t)

	zeroDecimal := decimal.New(0, 0)

	user, err := runEnv.Create(context.Background(), Request{userLogin, userPassword})
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &User{}, user)
	assert.Equal(t, userLogin, user.Login)
	assert.Equal(t, HashPassword(userPassword), user.Password)
	assert.Equal(t, zeroDecimal, user.Balance)
	assert.Equal(t, zeroDecimal, user.Withdrawn)

	user = nil
	user, err = runEnv.GetByLogin(context.Background(), userLogin)
	assert.Nilf(t, err, "order get failed with error %s", err)
	assert.IsType(t, &User{}, user)
	assert.Equal(t, userLogin, user.Login)
	assert.Equal(t, HashPassword(userPassword), user.Password)
	assert.Equal(t, zeroDecimal, user.Balance)
	assert.Equal(t, zeroDecimal, user.Withdrawn)
}
