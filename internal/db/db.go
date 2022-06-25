package db

import (
	"context"

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

	db = pg{pgpool}

	_, err = db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
	id			serial PRIMARY KEY,
	login 		text NOT NULL,
	password	text NOT NULL
)`)
	if err != nil {
		return
	}

	_, err = db.Exec(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS \"users_login_idx\" ON users (login)")
	if err != nil {
		return
	}

	return db, nil
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
		return true, nil
	}
	return false, rows.Err()
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
