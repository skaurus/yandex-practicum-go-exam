package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"

	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
	"github.com/spf13/viper"
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

// if we used `type rfc3339Time time.Time`, we would not be able to call any time.Time
// method on values of this type and would be forced to cast them back and forth
type rfc3339Time struct {
	time.Time
}

func (t rfc3339Time) MarshalJSON() ([]byte, error) {
	return []byte("\"" + t.Format(time.RFC3339) + "\""), nil
}

func (t *rfc3339Time) Scan(src interface{}) error {
	switch v := src.(type) {
	case time.Time:
		*t = rfc3339Time{v}
	default:
		return errors.New("incompatible type for rfc3339Time")
	}
	return nil
}

type TransactionType string

const (
	TransactionDebit  TransactionType = "debit"
	TransactionCredit TransactionType = "credit"
)

type Transaction struct {
	ID          uint32           `json:"-"`
	UserID      uint32           `json:"-"`
	OrderNumber string           `json:"order"`
	ProcessedAt rfc3339Time      `json:"processed_at"`
	Operation   TransactionType  `json:"-"`
	Value       *decimal.Decimal `json:"sum"`
}

func (runEnv Env) AddTransaction(ctx context.Context, userID int, orderNumber string, operation TransactionType, sum *decimal.Decimal) (t *Transaction, err error) {
	t = &Transaction{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
	defer cancel()
	_, err = runEnv.DB().QueryRow(
		ctx,
		t,
		`
INSERT INTO ledger (user_id, order_number, operation, value)
VALUES ($1, $2::bigint, $3, $4)
RETURNING id, user_id, order_number::text, processed_at, operation, value
`,
		userID, orderNumber, operation, sum,
	)
	// If err was returned - it will end up in that return; there can't be a
	// conflict, so t should never be nil when err is also nil. Overall it
	// means no further processing of the answer is required.
	return
}

func (runEnv Env) GetListByUserID(ctx context.Context, userID int) (ts *[]Transaction, err error) {
	ts = &[]Transaction{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
	defer cancel()
	err = runEnv.DB().QueryAll(
		ctx,
		ts,
		`
SELECT id, user_id, order_number::text, processed_at, operation, value
FROM ledger
WHERE user_id = $1
ORDER BY processed_at ASC
`,
		userID,
	)
	return
}

func (runEnv Env) GetList(ctx context.Context, where string, orderBy string, args ...interface{}) (ts *[]Transaction, err error) {
	ts = &[]Transaction{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
	defer cancel()
	err = runEnv.DB().QueryAll(
		ctx,
		ts,
		fmt.Sprintf(`
SELECT id, user_id, order_number::text, processed_at, operation, value
FROM ledger
WHERE %s
%s
`, where, orderBy),
		args...,
	)
	return
}
