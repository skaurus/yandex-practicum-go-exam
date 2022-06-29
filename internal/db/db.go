package db

import (
	"context"
	"time"

	"github.com/georgysavva/scany/pgxscan"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/spf13/viper"
)

type DB interface {
	Exec(context.Context, string, ...interface{}) (int64, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, interface{}, string, ...interface{}) (bool, error)
	QueryAll(context.Context, interface{}, string, ...interface{}) error
	Close()
	InitSchema(ctx context.Context) error
}

type pg struct {
	handle *pgxpool.Pool
}

func Connect(ctx context.Context) (db DB, err error) {
	connConfig, err := pgxpool.ParseConfig(viper.GetString("DATABASE_URI"))
	if err != nil {
		return pg{}, err
	}
	pgpool, err := pgxpool.ConnectConfig(ctx, connConfig)
	if err != nil {
		return pg{}, err
	}

	return pg{pgpool}, nil
}

func (db pg) Close() {
	db.handle.Close()
}

func (db pg) Exec(ctx context.Context, query string, args ...interface{}) (int64, error) {
	comTag, err := db.handle.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return comTag.RowsAffected(), nil
}

func (db pg) Query(ctx context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	return db.handle.Query(ctx, query, args...)
}

func (db pg) QueryRow(ctx context.Context, dst interface{}, query string, args ...interface{}) (found bool, err error) {
	rows, err := db.handle.Query(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		if err = pgxscan.ScanRow(dst, rows); err != nil {
			return
		}
		found = true
	}
	return found, rows.Err()
}

func (db pg) QueryAll(ctx context.Context, dst interface{}, query string, args ...interface{}) error {
	rows, err := db.handle.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	err = pgxscan.ScanAll(dst, rows)
	if err != nil {
		return err
	}

	return rows.Err()
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
		CREATE TYPE order_status AS enum ('NEW','PROCESSING','INVALID','PROCESSED');
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
	id			serial PRIMARY KEY,
	user_id		integer NOT NULL,
	added_at	timestamp NOT NULL DEFAULT now(),
	status		order_status NOT NULL
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
CREATE INDEX IF NOT EXISTS "orders_user_id_added_at_idx" ON orders (user_id, added_at ASC)
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
	processed_at  	timestamp NOT NULL DEFAULT now(),
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
