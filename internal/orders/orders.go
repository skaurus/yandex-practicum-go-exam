package orders

import (
	"context"
	"errors"
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
	Status     Status           `json:"Status"`
	Accrual    *decimal.Decimal `json:"accrual,omitempty"`
}

func (runEnv Env) Create(ctx context.Context, number string, userID int) (o *Order, err error) {
	o = &Order{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
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

func (runEnv Env) GetByNumber(ctx context.Context, number string) (o *Order, err error) {
	o = &Order{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
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

func (runEnv Env) GetListByUserID(ctx context.Context, userID int) (os *[]Order, err error) {
	os = &[]Order{}
	ctx, cancel := context.WithTimeout(ctx, viper.Get("DB_QUERY_TIMEOUT").(time.Duration))
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
