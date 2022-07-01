package users

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"
	"github.com/skaurus/yandex-practicum-go-exam/internal/ledger"

	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
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
	ID        uint32
	Login     string
	Password  string
	Balance   decimal.Decimal
	Withdrawn decimal.Decimal
}

type Request struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (runEnv Env) Create(ctx context.Context, req Request) (u *User, err error) {
	u = &User{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
	defer cancel()
	_, err = runEnv.DB().QueryRow(
		ctx,
		u,
		`
INSERT INTO users (login, password)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
RETURNING id, login, password, balance, withdrawn
`,
		req.Login, HashPassword(req.Password),
	)
	// If err was returned - it will end up in that return; if there was conflict
	// (meaning that login is taken) - u will be nil. It means no further
	// processing of the answer is required.
	return
}

func (runEnv Env) GetByLogin(ctx context.Context, login string) (u *User, err error) {
	u = &User{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
	defer cancel()
	_, err = runEnv.DB().QueryRow(
		ctx,
		u,
		"SELECT id, login, password, balance, withdrawn FROM users WHERE login = $1",
		login,
	)
	// If err was returned - it will end up in that return; if the missing return
	// argument (found) is false - then u will be nil. It means no further
	// processing of the answer is required.
	return
}

func (runEnv Env) GetByID(ctx context.Context, id int) (u *User, err error) {
	u = &User{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
	defer cancel()
	_, err = runEnv.DB().QueryRow(
		ctx,
		u,
		"SELECT id, login, password, balance, withdrawn FROM users WHERE id = $1",
		id,
	)
	// If err was returned - it will end up in that return; if the missing return
	// argument (found) is false - then u will be nil. It means no further
	// processing of the answer is required.
	return
}

var ErrNoSuchUser = errors.New("no such user")
var ErrInsufficientFunds = errors.New("insufficient funds")

func (runEnv Env) Withdraw(ctx context.Context, ledgerEnv ledger.Env, userID int, OrderNumber int, sum decimal.Decimal) error {
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
	defer cancel()

	return runEnv.DB().Transaction(ctx, func(ctx context.Context, db db.DB) error {
		balance := &decimal.Decimal{}
		_, err := db.QueryRow(
			ctx,
			balance,
			"SELECT balance FROM users WHERE id = $1",
			userID,
		)
		if err != nil {
			return err
		}
		if balance == nil {
			return ErrNoSuchUser
		}

		if balance.LessThan(sum) {
			return ErrInsufficientFunds
		}

		rowsAffected, err := db.Exec(
			ctx,
			// second condition in WHERE - to just be 100% sure
			"UPDATE users SET balance = balance - $1, withdrawn = withdrawn + $1 WHERE id = $2 AND balance >= $1",
			sum, userID,
		)
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return ErrInsufficientFunds
		}

		_, err = ledgerEnv.AddTransaction(ctx, userID, OrderNumber, ledger.TransactionCredit, sum)
		if err != nil {
			return err
		}

		return nil
	})
}

func HashPassword(password string) string {
	// Gentle Argon2id settings are used to be merciful on testing container.
	// In production, memory should be increased to say 64MB.
	hashedBytes := argon2.IDKey(
		[]byte(password),
		[]byte(viper.Get("PASSWORD_SECRET").(string)),
		1,
		16*1024, // 16MB
		2,
		32,
	)
	// 1: prefix is used to be able to later introduce another hashing schemes.
	return "1:" + base64.StdEncoding.EncodeToString(hashedBytes)
}

func (runEnv Env) CheckPassword(u *User, password string) bool {
	return u.Password == HashPassword(password)
}
