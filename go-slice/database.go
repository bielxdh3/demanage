package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresDatabase struct {
	databaseURL string
	once        sync.Once
	poolValue   *pgxpool.Pool
	poolError   error
}

func newPostgresDatabase(databaseURL string) *postgresDatabase {
	return &postgresDatabase{databaseURL: databaseURL}
}

func (database *postgresDatabase) pool() (*pgxpool.Pool, error) {
	database.once.Do(func() {
		if database.databaseURL == "" {
			database.poolError = errors.New("database is not configured")
			return
		}
		config, err := pgxpool.ParseConfig(database.databaseURL)
		if err != nil {
			database.poolError = err
			return
		}
		config.MaxConns = 4
		database.poolValue, database.poolError = pgxpool.NewWithConfig(context.Background(), config)
	})
	return database.poolValue, database.poolError
}

func (database *postgresDatabase) Probe(requestContext context.Context) error {
	ctx, cancel := context.WithTimeout(requestContext, 2*time.Second)
	defer cancel()
	pool, err := database.pool()
	if err != nil {
		return err
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()
	_, err = connection.Exec(ctx, "SELECT 1")
	return err
}

func (database *postgresDatabase) Close() {
	if database.poolValue != nil {
		database.poolValue.Close()
	}
}
