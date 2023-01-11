package orders

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

var orderNumberOne = "-1"
var orderNumberTwo = "-2"
var userNumberOne uint32 = 1
var userNumberTwo uint32 = 2

func getTestEnv(t *testing.T) localEnv {
	err := viper.BindEnv("DATABASE_URI", "DATABASE_URI")
	assert.Nilf(t, err, "viper.BindEnv has failed: %v", err)
	db.QueryTimeout = time.Second

	db, err := db.Connect(context.Background())
	assert.Nilf(t, err, "connect has failed with error %v", err)
	assert.NotNil(t, db)
	assert.NotNil(t, db.Handle())
	assert.IsType(t, &pgxpool.Pool{}, db.Handle())

	_, err = db.Exec(context.Background(), "DELETE FROM orders WHERE number < 0")
	assert.Nilf(t, err, "connect has failed with error %v", err)

	zlog := zerolog.New(os.Stdout)
	env := env.Init(db, &zlog)
	return localEnv{&env}
}

func Test_CreateAndGet(t *testing.T) {
	runEnv := getTestEnv(t)

	var nilDecimal *decimal.Decimal

	order, err := runEnv.Create(context.Background(), orderNumberOne, int(userNumberOne))
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Order{}, order)
	assert.Equal(t, orderNumberOne, order.Number)
	assert.Equal(t, userNumberOne, order.UserID)
	assert.Equal(t, StatusNew, order.Status)
	assert.Equal(t, nilDecimal, order.Accrual)

	order = nil
	order, err = runEnv.GetByNumber(context.Background(), orderNumberOne)
	assert.Nilf(t, err, "order get failed with error %s", err)
	assert.IsType(t, &Order{}, order)
	assert.Equal(t, orderNumberOne, order.Number)
	assert.Equal(t, userNumberOne, order.UserID)
	assert.Equal(t, StatusNew, order.Status)
	assert.Equal(t, nilDecimal, order.Accrual)
}

func Test_CreateAndListByUserID(t *testing.T) {
	runEnv := getTestEnv(t)

	order1, err := runEnv.Create(context.Background(), orderNumberOne, int(userNumberTwo))
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Order{}, order1)
	assert.Equal(t, orderNumberOne, order1.Number)
	assert.Equal(t, userNumberTwo, order1.UserID)

	order2, err := runEnv.Create(context.Background(), orderNumberTwo, int(userNumberTwo))
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Order{}, order2)
	assert.Equal(t, orderNumberTwo, order2.Number)
	assert.Equal(t, userNumberTwo, order2.UserID)

	orders, err := runEnv.GetListByUserID(context.Background(), int(userNumberTwo))
	assert.Nilf(t, err, "orders list failed with error %s", err)
	assert.IsType(t, &[]Order{}, orders)
	assert.Equal(t, 2, len(*orders))
	assert.Equal(t, orderNumberOne, (*orders)[0].Number)
	assert.Equal(t, userNumberTwo, (*orders)[0].UserID)
	assert.Equal(t, orderNumberTwo, (*orders)[1].Number)
	assert.Equal(t, userNumberTwo, (*orders)[1].UserID)
}

func Test_CreateAndList(t *testing.T) {
	runEnv := getTestEnv(t)

	order1, err := runEnv.Create(context.Background(), orderNumberOne, int(userNumberOne))
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Order{}, order1)
	assert.Equal(t, orderNumberOne, order1.Number)
	assert.Equal(t, userNumberOne, order1.UserID)

	order2, err := runEnv.Create(context.Background(), orderNumberTwo, int(userNumberOne))
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Order{}, order2)
	assert.Equal(t, orderNumberTwo, order2.Number)
	assert.Equal(t, userNumberOne, order2.UserID)

	orders, err := runEnv.GetList(
		context.Background(),
		"number < 0 AND status = $1",
		"ORDER BY uploaded_at DESC",
		StatusNew,
	)
	assert.Nilf(t, err, "orders list failed with error %s", err)
	assert.IsType(t, &[]Order{}, orders)
	assert.Equal(t, 2, len(*orders))
	assert.Equal(t, orderNumberTwo, (*orders)[0].Number)
	assert.Equal(t, userNumberOne, (*orders)[0].UserID)
	assert.Equal(t, orderNumberOne, (*orders)[1].Number)
	assert.Equal(t, userNumberOne, (*orders)[1].UserID)

	orders, err = runEnv.GetList(
		context.Background(),
		"number < 0 AND status != $1",
		"ORDER BY uploaded_at DESC",
		StatusNew,
	)
	assert.Nilf(t, err, "orders list failed with error %s", err)
	assert.IsType(t, &[]Order{}, orders)
	assert.Equal(t, 0, len(*orders))
}

func Test_CreateUpdateAndGet(t *testing.T) {
	runEnv := getTestEnv(t)

	order, err := runEnv.Create(context.Background(), orderNumberTwo, int(userNumberTwo))
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Order{}, order)
	assert.Equal(t, orderNumberTwo, order.Number)
	assert.Equal(t, userNumberTwo, order.UserID)

	order.Status = StatusInvalid
	dec, _ := decimal.NewFromString("0.42")
	order.Accrual = &dec
	rowsAffected, err := runEnv.Update(context.Background(), OrderUpdate{order.Number, order.Status, order.Accrual})
	assert.Nilf(t, err, "order get failed with error %s", err)
	assert.Equal(t, 1, rowsAffected)

	order = nil
	order, err = runEnv.GetByNumber(context.Background(), orderNumberTwo)
	assert.Nilf(t, err, "order get failed with error %s", err)
	assert.IsType(t, &Order{}, order)
	assert.Equal(t, orderNumberTwo, order.Number)
	assert.Equal(t, userNumberTwo, order.UserID)
	assert.Equal(t, StatusInvalid, order.Status)
	assert.IsType(t, &decimal.Decimal{}, order.Accrual)
	assert.Equal(t, dec, *order.Accrual)
}
