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
	"github.com/skaurus/yandex-practicum-go-exam/internal/ledger"
	"github.com/skaurus/yandex-practicum-go-exam/internal/orders"
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

func Test_AccrueAndWithdraw(t *testing.T) {
	usersEnv := getTestEnv(t)
	globalEnv := env.Init(usersEnv.DB(), usersEnv.Logger())
	ordersEnv := orders.Env{Env: &globalEnv}
	ledgerEnv := ledger.Env{Env: &globalEnv}

	orderNumber := "-1"
	_, err := usersEnv.DB().Exec(context.Background(), "DELETE FROM orders WHERE number::text = $1", orderNumber)
	assert.Nilf(t, err, "cleaning up db has failed with error %v", err)

	accrual := decimal.New(2*42, 0)
	withdraw := decimal.New(1*42, 0)

	order := &orders.Order{
		Number:  orderNumber,
		UserID:  1,
		Status:  orders.StatusProcessed,
		Accrual: &accrual,
	}

	err = ordersEnv.Accrue(context.Background(), ledgerEnv, order)
	assert.ErrorContains(t, err, orders.ErrNoSuchOrder.Error())

	err = usersEnv.Withdraw(context.Background(), ledgerEnv, int(order.UserID), orderNumber, &withdraw)
	assert.ErrorContains(t, err, ErrNoSuchUser.Error())

	order, err = ordersEnv.Create(context.Background(), orderNumber, 1)
	assert.Nil(t, err)
	assert.IsType(t, &orders.Order{}, order)
	assert.Equal(t, orderNumber, order.Number)

	rowsAffected, err := ordersEnv.Update(
		context.Background(),
		orders.OrderUpdate{
			Number:  orderNumber,
			Status:  orders.StatusProcessed,
			Accrual: &accrual,
		},
	)
	assert.Nil(t, err)
	assert.Equal(t, 1, rowsAffected)

	order, err = ordersEnv.GetByNumber(context.Background(), orderNumber)
	assert.Nil(t, err)
	assert.IsType(t, &orders.Order{}, order)
	assert.Equal(t, orderNumber, order.Number)
	assert.Equal(t, orders.StatusProcessed, order.Status)
	assert.True(t, order.Accrual.Equal(accrual))

	err = ordersEnv.Accrue(context.Background(), ledgerEnv, order)
	assert.ErrorContains(t, err, ErrNoSuchUser.Error())

	err = usersEnv.Withdraw(context.Background(), ledgerEnv, int(order.UserID), orderNumber, &withdraw)
	assert.ErrorContains(t, err, ErrNoSuchUser.Error())

	user, err := usersEnv.Create(context.Background(), Request{"", ""})
	assert.Nil(t, err)
	assert.IsType(t, &User{}, user)
	assert.True(t, user.ID > 0)

	order.UserID = user.ID

	err = ordersEnv.Accrue(context.Background(), ledgerEnv, order)
	assert.Nil(t, err)

	transactions, err := ledgerEnv.GetList(
		context.Background(),
		"order_number::text = $1 AND user_id = $2",
		"",
		order.Number, user.ID,
	)
	assert.Nil(t, err)
	assert.IsType(t, &[]ledger.Transaction{}, transactions)
	assert.Equal(t, 1, len(*transactions))
	transaction := (*transactions)[0]
	assert.Equal(t, ledger.TransactionDebit, transaction.Operation)
	assert.True(t, transaction.Value.Equal(accrual))

	user, err = usersEnv.GetByID(context.Background(), int(user.ID))
	assert.Nil(t, err)
	assert.Equal(t, order.UserID, user.ID)
	assert.True(t, user.Balance.Equal(accrual))
	assert.True(t, user.Withdrawn.Equal(decimal.New(0, 0)))

	err = usersEnv.Withdraw(context.Background(), ledgerEnv, int(order.UserID), orderNumber, &withdraw)
	assert.Nil(t, err)

	transactions, err = ledgerEnv.GetList(
		context.Background(),
		"order_number::text = $1 AND user_id = $2",
		"ORDER BY processed_at ASC",
		order.Number, user.ID,
	)
	assert.Nil(t, err)
	assert.IsType(t, &[]ledger.Transaction{}, transactions)
	assert.Equal(t, 2, len(*transactions))
	assert.Equal(t, ledger.TransactionDebit, (*transactions)[0].Operation)
	assert.True(t, (*transactions)[0].Value.Equal(accrual))
	assert.Equal(t, ledger.TransactionCredit, (*transactions)[1].Operation)
	assert.True(t, (*transactions)[1].Value.Equal(withdraw))

	user, err = usersEnv.GetByID(context.Background(), int(user.ID))
	assert.Nil(t, err)
	assert.Equal(t, order.UserID, user.ID)
	assert.True(t, user.Balance.Equal(accrual.Sub(withdraw)))
	assert.True(t, user.Withdrawn.Equal(withdraw))
}
