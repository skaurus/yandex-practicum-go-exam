package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/skaurus/yandex-practicum-go-exam/internal/db"
	"github.com/skaurus/yandex-practicum-go-exam/internal/env"
	"github.com/skaurus/yandex-practicum-go-exam/internal/ledger"

	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

const modelName = "orders"

type localEnv struct {
	env *env.Env
}

func (runEnv localEnv) DB() db.DB {
	return runEnv.env.DB()
}

func (runEnv localEnv) Logger() *zerolog.Logger {
	return runEnv.env.Logger()
}

func InitEnv(runEnv *env.Env) error {
	return env.InitModelEnv(modelName, localEnv{env: runEnv})
}

func GetEnv() localEnv {
	return env.GetEnv(modelName).(localEnv)
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

type Status string

const (
	StatusNew        Status = "NEW"
	StatusRegistered Status = "REGISTERED"
	StatusProcessing Status = "PROCESSING"
	StatusInvalid    Status = "INVALID"
	StatusProcessed  Status = "PROCESSED"
)

type Order struct {
	Number     string           `json:"number"`
	UserID     uint32           `json:"-"`
	UploadedAt rfc3339Time      `json:"uploaded_at"`
	Status     Status           `json:"status"`
	Accrual    *decimal.Decimal `json:"accrual,omitempty"`
}

type Create struct {
	Number string
	UserID uint32
}

func (runEnv localEnv) Create(ctx context.Context, number string, userID int) (o *Order, err error) {
	o = &Order{}
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()
	_, err = runEnv.DB().QueryRow(
		ctx,
		o,
		`
INSERT INTO orders (number, user_id, status)
VALUES ($1::bigint, $2, $3)
ON CONFLICT DO NOTHING
RETURNING number::text, user_id, uploaded_at, status, accrual
`,
		number, userID, StatusNew,
	)
	// If err was returned - it will end up in that return; if there was conflict
	// (meaning that order already exists) - o will be nil. It means no further
	// processing of the answer is required.
	return
}

func (runEnv localEnv) GetByNumber(ctx context.Context, number string) (o *Order, err error) {
	o = &Order{}
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()
	_, err = runEnv.DB().QueryRow(
		ctx,
		o,
		"SELECT number::text, user_id, uploaded_at, status, accrual FROM orders WHERE number = $1::bigint",
		number,
	)
	// If err was returned - it will end up in that return; if the missing return
	// argument (found) is false - then o will be nil. It means no further
	// processing of the answer is required.
	return
}

type OrderUpdate struct {
	Number  string
	Status  Status
	Accrual *decimal.Decimal
}

func (runEnv localEnv) Update(ctx context.Context, o OrderUpdate) (rowsAffected int, err error) {
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()
	return runEnv.DB().Exec(
		ctx,
		"UPDATE orders SET status = $1, accrual = $2 WHERE number = $3::bigint",
		o.Status, o.Accrual, o.Number,
	)
}

func (runEnv localEnv) GetListByUserID(ctx context.Context, userID int) (os *[]Order, err error) {
	os = &[]Order{}
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()
	err = runEnv.DB().QueryAll(
		ctx,
		os,
		`
SELECT number::text, user_id, uploaded_at, status, accrual
FROM orders
WHERE user_id = $1
ORDER BY uploaded_at ASC
`,
		userID,
	)
	return
}

func (runEnv localEnv) GetList(ctx context.Context, where string, orderBy string, args ...interface{}) (os *[]Order, err error) {
	os = &[]Order{}
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()
	err = runEnv.DB().QueryAll(
		ctx,
		os,
		fmt.Sprintf(`
SELECT number::text, user_id, uploaded_at, status, accrual
FROM orders
WHERE %s
%s
`, where, orderBy),
		args...,
	)
	return
}

var ErrNoSuchOrder = errors.New("no such order")
var ErrNoSuchUser = errors.New("no such user")

func (runEnv localEnv) Accrue(ctx context.Context, o *Order) error {
	ctx, cancel := context.WithTimeout(ctx, db.QueryTimeout)
	defer cancel()

	return runEnv.DB().Transaction(ctx, func(ctx context.Context, db db.DB) error {
		rowsAffected, err := runEnv.Update(ctx, OrderUpdate{o.Number, o.Status, o.Accrual})
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return ErrNoSuchOrder
		}

		rowsAffected, err = db.Exec(
			ctx,
			// second condition in WHERE - just to be 100% sure
			"UPDATE users SET balance = balance + $1 WHERE id = $2",
			o.Accrual, o.UserID,
		)
		if err != nil {
			return err
		}
		if rowsAffected == 0 {
			return ErrNoSuchUser
		}

		// This is not DB transaction, it's a record in a lender
		_, err = ledger.GetEnv().AddTransaction(ctx, int(o.UserID), o.Number, ledger.TransactionDebit, o.Accrual)
		if err != nil {
			return err
		}

		return nil
	})
}
