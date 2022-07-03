package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgtype"
	shopspring "github.com/jackc/pgtype/ext/shopspring-numeric"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/spf13/viper"
)

// What i'm trying to do here is bring some unified interface for pgxpool.Pool
// and pgx.Tx, so that code can do not think about whether now it should call
// one of these db methods on pool connection or transaction descriptor.
// For some reason, there is no such thing from jackc himself.
// See also Handle() and MustTx().
type queryMaker interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
}

type DB interface {
	Handle() queryMaker
	Exec(context.Context, string, ...interface{}) (int, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, interface{}, string, ...interface{}) (bool, error)
	QueryAll(context.Context, interface{}, string, ...interface{}) error
	Transaction(context.Context, func(context.Context, DB) error) error
	MustTx() pgx.Tx // WARNING - will panic if currently not in a transaction
	Rollback(context.Context) error
	Close()
	InitSchema(context.Context) error
}

type pg struct {
	conn *pgxpool.Pool
	tx   *pgx.Tx
}

func Connect(ctx context.Context) (db DB, err error) {
	connConfig, err := pgxpool.ParseConfig(viper.GetString("DATABASE_URI"))
	if err != nil {
		return pg{}, err
	}

	connConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		conn.ConnInfo().RegisterDataType(pgtype.DataType{
			Value: &shopspring.Numeric{},
			Name:  "numeric",
			OID:   pgtype.NumericOID,
		})
		return nil
	}

	pgpool, err := pgxpool.ConnectConfig(ctx, connConfig)
	if err != nil {
		return pg{}, err
	}

	return pg{pgpool, nil}, nil
}

func (db pg) Close() {
	db.conn.Close()
}

var ErrNestedTransaction = errors.New("nested transactions are not supported")
var ErrNotInATransaction = errors.New("not in a transaction now")

func (db pg) Transaction(ctx context.Context, doWork func(context.Context, DB) error) error {
	if db.tx != nil {
		return ErrNestedTransaction
	}

	tx, err := db.conn.Begin(ctx)
	if err != nil {
		return err
	}
	db.tx = &tx
	defer db.Rollback(ctx)

	err = doWork(ctx, db)
	if err != nil {
		rollbackErr := db.Rollback(ctx)
		if rollbackErr != nil {
			err = fmt.Errorf(
				"rollback failed with error: %s; rollback itself was initiated after error: %s",
				rollbackErr, err,
			)
		}
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (db pg) Handle() queryMaker {
	if db.tx == nil {
		return db.conn
	} else {
		return *db.tx
	}
}

func (db pg) MustTx() pgx.Tx {
	if db.tx == nil {
		panic(ErrNotInATransaction)
	}
	return *db.tx
}

func (db pg) Rollback(ctx context.Context) error {
	defer func() { db.tx = nil }()

	return db.MustTx().Rollback(ctx)
}

func (db pg) Exec(ctx context.Context, query string, args ...interface{}) (int, error) {
	comTag, err := db.Handle().Exec(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("error [%w] running query: %s", err, query)
	}
	return int(comTag.RowsAffected()), nil
}

func (db pg) Query(ctx context.Context, query string, args ...interface{}) (rows pgx.Rows, err error) {
	rows, err = db.Handle().Query(ctx, query, args...)
	if err != nil {
		err = fmt.Errorf("error [%w] running query: %s", err, query)
	}
	return
}

func (db pg) QueryRow(ctx context.Context, dst interface{}, query string, args ...interface{}) (found bool, err error) {
	rows, err := db.Handle().Query(ctx, query, args...)
	if err != nil {
		err = fmt.Errorf("error [%w] running query: %s", err, query)
		return
	}
	defer rows.Close()

	if rows.Next() {
		if err = pgxscan.ScanRow(dst, rows); err != nil {
			err = fmt.Errorf("error [%w] reading query: %s", err, query)
			return
		}
		found = true
	}

	err = rows.Err()
	if err != nil {
		err = fmt.Errorf("error [%w] running query: %s", err, query)
	}
	return
}

func (db pg) QueryAll(ctx context.Context, dst interface{}, query string, args ...interface{}) error {
	rows, err := db.Handle().Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("error [%w] running query: %s", err, query)
	}
	defer rows.Close()
	err = pgxscan.ScanAll(dst, rows)
	if err != nil {
		return fmt.Errorf("error [%w] reading query: %s", err, query)
	}

	err = rows.Err()
	if err != nil {
		err = fmt.Errorf("error [%w] running query: %s", err, query)
	}
	return err
}

// InitSchema - probably it would be best to use some database migration
// system - most of all, not for the ability to rollback a schema (it's better
// to have documented deploy procedures IMO), but for a history of changes.
func (db pg) InitSchema(origCtx context.Context) error {
	ctx, cancel := context.WithTimeout(
		origCtx,
		viper.Get("DB_QUERY_TIMEOUT").(time.Duration),
	)
	_, err := db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
	id			serial PRIMARY KEY,
	login 		text NOT NULL,
	password	text NOT NULL,
	balance     numeric(8,2) NOT NULL DEFAULT 0,
	withdrawn	numeric(8,2) NOT NULL DEFAULT 0
)`)
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(
		origCtx,
		viper.Get("DB_QUERY_TIMEOUT").(time.Duration),
	)
	_, err = db.Exec(ctx, `
CREATE UNIQUE INDEX IF NOT EXISTS "users_login_idx" ON users (login)
`)
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(
		origCtx,
		viper.Get("DB_QUERY_TIMEOUT").(time.Duration),
	)
	_, err = db.Exec(ctx, `
DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'order_status') THEN
		CREATE TYPE order_status AS enum ('NEW','REGISTERED','PROCESSING','INVALID','PROCESSED');
	END IF;
END$$
`)
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(
		origCtx,
		viper.Get("DB_QUERY_TIMEOUT").(time.Duration),
	)
	_, err = db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS orders (
	number		bigint PRIMARY KEY,
	user_id		integer NOT NULL,
	uploaded_at	timestamp with time zone NOT NULL DEFAULT now(),
	status		order_status NOT NULL,
	accrual 	numeric(8,2)
)`)
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(
		origCtx,
		viper.Get("DB_QUERY_TIMEOUT").(time.Duration),
	)
	_, err = db.Exec(ctx, `
CREATE INDEX IF NOT EXISTS "orders_user_id_uploaded_at_idx" ON orders (user_id, uploaded_at ASC)
`)
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(
		origCtx,
		viper.Get("DB_QUERY_TIMEOUT").(time.Duration),
	)
	// Specification does not require me to store a debit operations, but this
	// is a very useful feature. For example, we can always recheck user balance.
	_, err = db.Exec(ctx, `
DO $$
BEGIN
	IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'transaction_type') THEN
		CREATE TYPE transaction_type AS enum ('debit','credit');
	END IF;
END$$
`)
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(
		origCtx,
		viper.Get("DB_QUERY_TIMEOUT").(time.Duration),
	)
	_, err = db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS ledger (
	id				serial PRIMARY KEY,
	user_id 		integer NOT NULL,
	order_number 	bigint NOT NULL,
	processed_at  	timestamp with time zone NOT NULL DEFAULT now(),
	operation 		transaction_type NOT NULL,
	value     		numeric(8,2) NOT NULL
)`)
	cancel()
	if err != nil {
		return err
	}

	ctx, cancel = context.WithTimeout(
		origCtx,
		viper.Get("DB_QUERY_TIMEOUT").(time.Duration),
	)
	_, err = db.Exec(ctx, `
CREATE INDEX IF NOT EXISTS "ledger_user_id_processed_at_idx" ON ledger (user_id, processed_at ASC)
`)
	cancel()
	if err != nil {
		return err
	}

	return nil
}
