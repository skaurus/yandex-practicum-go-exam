package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type testRow struct {
	ID  int
	Msg string
}

var oneString, secondString string = "one string", "another string"

var db DB

func initDBConnection(t *testing.T) DB {
	// Since tests can be run separately, TRUNCATE is needed every time so state is known
	if db != nil {
		// Obviously we have a race condition if we have tests running on the same
		// db at the same time. One solution is to mock DB, but that won't get us
		// far in testing db code... Another is that each developer have separate
		// testing environment.
		_, err := db.Exec(context.Background(), "TRUNCATE TABLE test")
		assert.Nilf(t, err, "TRUNCATE has failed with error %v", err)
		return db
	}

	err := viper.BindEnv("DATABASE_URI", "DATABASE_URI")
	assert.Nilf(t, err, "viper.BindEnv has failed: %v", err)

	db, err = Connect(context.Background())
	assert.Nilf(t, err, "connect has failed with error %v", err)
	assert.NotNil(t, db)
	assert.NotNil(t, db.Handle())
	assert.IsType(t, &pgxpool.Pool{}, db.Handle())

	rowsAffected, err := db.Exec(
		context.Background(),
		"CREATE TABLE IF NOT EXISTS test(id int PRIMARY KEY NOT NULL, msg text)",
	)
	assert.Nilf(t, err, "CREATE TABLE has failed with error %v", err)
	assert.Equal(t, 0, rowsAffected)

	// Obviously we have a race condition if we have tests running on the same
	// db at the same time. One solution is to mock DB, but that won't get us
	// far in testing db code... Another is that each developer have separate
	// testing environment.
	_, err = db.Exec(context.Background(), "TRUNCATE TABLE test")
	assert.Nilf(t, err, "TRUNCATE has failed with error %v", err)

	return db
}

func Test_Transaction(t *testing.T) {
	db := initDBConnection(t)

	assert.Panics(t, func() { db.MustTx() })
	err := db.Transaction(context.Background(), func(ctx context.Context, db DB) (err error) {
		assert.NotPanics(t, func() { db.MustTx() })
		var tt *pgxpool.Tx
		assert.IsType(t, tt, db.Handle())

		err = db.Transaction(ctx, func(ctx context.Context, db DB) error {
			return nil
		})
		assert.ErrorIs(t, err, ErrNestedTransaction)

		rowsAffected, err := db.Exec(ctx, "INSERT INTO test (id, msg) VALUES (1, $1), (2, $2)", oneString, secondString)
		assert.Nilf(t, err, "INSERT has failed with error %v", err)
		assert.Equal(t, 2, rowsAffected)

		var cnt int
		found, err := db.QueryRow(ctx, &cnt, "SELECT count(*) FROM test")
		assert.Equal(t, true, found)
		assert.Nilf(t, err, "SELECT count(*) has failed with error %v", err)
		assert.Equal(t, 2, cnt)

		_, err = db.Exec(ctx, "SELECT 1/0")
		assert.NotNil(t, err)
		return err
	})
	assert.ErrorContains(t, err, "division by zero")

	var cnt int
	found, err := db.QueryRow(context.Background(), &cnt, "SELECT count(*) FROM test")
	assert.Equal(t, true, found)
	assert.Nilf(t, err, "SELECT count(*) has failed with error %v", err)
	assert.Equalf(t, 0, cnt, "table test is not empty. has transaction failed?")
}

func Test_Query(t *testing.T) {
	db := initDBConnection(t)

	rowsAffected, err := db.Exec(
		context.Background(),
		"INSERT INTO test (id, msg) VALUES (1, $1), (2, $2)",
		oneString, secondString,
	)
	assert.Nilf(t, err, "INSERT has failed with error %v", err)
	assert.Equal(t, 2, rowsAffected)

	rows, err := db.Query(context.Background(), "SELECT * FROM test ORDER BY id ASC")
	assert.Nilf(t, err, "SELECT count(*) has failed with error %v", err)
	var res []testRow
	for i := 0; rows.Next(); i++ {
		res = append(res, testRow{})
		err = rows.Scan(&res[i].ID, &res[i].Msg)
		assert.Nil(t, err)
	}
	assert.Nil(t, rows.Err())
	assert.Equal(t, 2, len(res))
	assert.Equal(t, 1, res[0].ID)
	assert.Equal(t, oneString, res[0].Msg)
	assert.Equal(t, 2, res[1].ID)
	assert.Equal(t, secondString, res[1].Msg)
}

func Test_QueryRow(t *testing.T) {
	db := initDBConnection(t)

	rowsAffected, err := db.Exec(
		context.Background(),
		"INSERT INTO test (id, msg) VALUES (1, $1), (2, $2)",
		oneString, secondString,
	)
	assert.Nilf(t, err, "INSERT has failed with error %v", err)
	assert.Equal(t, 2, rowsAffected)

	var row testRow
	found, err := db.QueryRow(context.Background(), &row, "SELECT * FROM test ORDER BY id ASC")
	assert.Equal(t, true, found)
	assert.Nilf(t, err, "SELECT count(*) has failed with error %v", err)
	assert.Equal(t, 1, row.ID)
	assert.Equal(t, oneString, row.Msg)
}

func Test_QueryAll(t *testing.T) {
	db := initDBConnection(t)

	rowsAffected, err := db.Exec(
		context.Background(),
		"INSERT INTO test (id, msg) VALUES (1, $1), (2, $2)",
		oneString, secondString,
	)
	assert.Nilf(t, err, "INSERT has failed with error %v", err)
	assert.Equal(t, 2, rowsAffected)

	var res []testRow
	err = db.QueryAll(context.Background(), &res, "SELECT * FROM test ORDER BY id DESC")
	assert.Nilf(t, err, "SELECT count(*) has failed with error %v", err)
	assert.Equal(t, 2, len(res))
	assert.Equal(t, 2, res[0].ID)
	assert.Equal(t, secondString, res[0].Msg)
	assert.Equal(t, 1, res[1].ID)
	assert.Equal(t, oneString, res[1].Msg)
}
