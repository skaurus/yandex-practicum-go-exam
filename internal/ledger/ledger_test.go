package ledger

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
var negativeDecimal = decimal.New(-42, 0)

func getTestEnv(t *testing.T) localEnv {
	err := viper.BindEnv("DATABASE_URI", "DATABASE_URI")
	assert.Nilf(t, err, "viper.BindEnv has failed: %v", err)
	db.QueryTimeout = time.Second

	db, err := db.Connect(context.Background())
	assert.Nilf(t, err, "connect has failed with error %v", err)
	assert.NotNil(t, db)
	assert.NotNil(t, db.Handle())
	assert.IsType(t, &pgxpool.Pool{}, db.Handle())

	_, err = db.Exec(context.Background(), "DELETE FROM ledger WHERE value < 0")
	assert.Nilf(t, err, "connect has failed with error %v", err)

	zlog := zerolog.New(os.Stdout)
	env := env.Init(db, &zlog)
	return localEnv{&env}
}

func Test_AddTransaction(t *testing.T) {
	runEnv := getTestEnv(t)

	transaction, err := runEnv.AddTransaction(
		context.Background(),
		int(userNumberOne), orderNumberOne,
		TransactionCredit, &negativeDecimal,
	)
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Transaction{}, transaction)
	assert.Equal(t, userNumberOne, transaction.UserID)
	assert.Equal(t, orderNumberOne, transaction.OrderNumber)
	assert.Equal(t, TransactionCredit, transaction.Operation)
	assert.True(t, negativeDecimal.Equal(*transaction.Value))

	transaction = nil
	transactions, err := runEnv.GetList(
		context.Background(),
		"user_id = $1 AND order_number = $2",
		"LIMIT 1",
		userNumberOne, orderNumberOne,
	)
	assert.Nilf(t, err, "order get failed with error %s", err)
	assert.Equal(t, 1, len(*transactions))
	transaction = &(*transactions)[0]
	assert.IsType(t, &Transaction{}, transaction)
	assert.Equal(t, userNumberOne, transaction.UserID)
	assert.Equal(t, orderNumberOne, transaction.OrderNumber)
	assert.Equal(t, TransactionCredit, transaction.Operation)
	assert.True(t, negativeDecimal.Equal(*transaction.Value))
}

func Test_CreateAndListByUserID(t *testing.T) {
	runEnv := getTestEnv(t)

	transaction1, err := runEnv.AddTransaction(
		context.Background(),
		int(userNumberOne), orderNumberOne,
		TransactionCredit, &negativeDecimal,
	)
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Transaction{}, transaction1)
	assert.Equal(t, userNumberOne, transaction1.UserID)
	assert.Equal(t, orderNumberOne, transaction1.OrderNumber)
	assert.Equal(t, TransactionCredit, transaction1.Operation)
	assert.True(t, negativeDecimal.Equal(*transaction1.Value))

	biggerNegativeDecimal := negativeDecimal.Mul(decimal.New(2, 0))
	transaction2, err := runEnv.AddTransaction(
		context.Background(),
		int(userNumberOne), orderNumberTwo,
		TransactionCredit, &biggerNegativeDecimal,
	)
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Transaction{}, transaction2)
	assert.Equal(t, userNumberOne, transaction2.UserID)
	assert.Equal(t, orderNumberTwo, transaction2.OrderNumber)
	assert.Equal(t, TransactionCredit, transaction2.Operation)
	assert.True(t, biggerNegativeDecimal.Equal(*transaction2.Value))

	transactions, err := runEnv.GetListByUserID(context.Background(), int(userNumberOne))
	assert.Nilf(t, err, "orders list failed with error %s", err)
	assert.IsType(t, &[]Transaction{}, transactions)
	assert.Equal(t, 2, len(*transactions))
	assert.Equal(t, orderNumberOne, (*transactions)[0].OrderNumber)
	assert.Equal(t, userNumberOne, (*transactions)[0].UserID)
	assert.Equal(t, TransactionCredit, (*transactions)[0].Operation)
	assert.True(t, negativeDecimal.Equal(*(*transactions)[0].Value))
	assert.Equal(t, orderNumberTwo, (*transactions)[1].OrderNumber)
	assert.Equal(t, userNumberOne, (*transactions)[1].UserID)
	assert.Equal(t, TransactionCredit, (*transactions)[1].Operation)
	assert.True(t, biggerNegativeDecimal.Equal(*(*transactions)[1].Value))
}

func Test_CreateAndList(t *testing.T) {
	runEnv := getTestEnv(t)

	transaction1, err := runEnv.AddTransaction(
		context.Background(),
		int(userNumberTwo), orderNumberOne,
		TransactionDebit, &negativeDecimal,
	)
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Transaction{}, transaction1)
	assert.Equal(t, userNumberTwo, transaction1.UserID)
	assert.Equal(t, orderNumberOne, transaction1.OrderNumber)
	assert.Equal(t, TransactionDebit, transaction1.Operation)
	assert.True(t, negativeDecimal.Equal(*transaction1.Value))

	biggerNegativeDecimal := negativeDecimal.Mul(decimal.New(2, 0))
	transaction2, err := runEnv.AddTransaction(
		context.Background(),
		int(userNumberTwo), orderNumberTwo,
		TransactionDebit, &biggerNegativeDecimal,
	)
	assert.Nilf(t, err, "order create failed with error %s", err)
	assert.IsType(t, &Transaction{}, transaction2)
	assert.Equal(t, userNumberTwo, transaction2.UserID)
	assert.Equal(t, orderNumberTwo, transaction2.OrderNumber)
	assert.Equal(t, TransactionDebit, transaction2.Operation)
	assert.True(t, biggerNegativeDecimal.Equal(*transaction2.Value))

	transactions, err := runEnv.GetList(
		context.Background(),
		"value < 0",
		"ORDER BY processed_at DESC",
	)
	assert.Nilf(t, err, "orders list failed with error %s", err)
	assert.IsType(t, &[]Transaction{}, transactions)
	assert.Equal(t, 2, len(*transactions))
	assert.Equal(t, orderNumberTwo, (*transactions)[0].OrderNumber)
	assert.Equal(t, userNumberTwo, (*transactions)[0].UserID)
	assert.Equal(t, TransactionDebit, (*transactions)[0].Operation)
	assert.True(t, biggerNegativeDecimal.Equal(*(*transactions)[0].Value))
	assert.Equal(t, orderNumberOne, (*transactions)[1].OrderNumber)
	assert.Equal(t, userNumberTwo, (*transactions)[1].UserID)
	assert.Equal(t, TransactionDebit, (*transactions)[1].Operation)
	assert.True(t, negativeDecimal.Equal(*(*transactions)[1].Value))
}
