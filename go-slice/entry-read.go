package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type postgresEntryRepository struct {
	database *postgresDatabase
}

func (repository postgresEntryRepository) ListEntries(ctx context.Context, userID string) ([]json.RawMessage, error) {
	if repository.database == nil {
		return nil, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, err
	}
	return listEntries(queryContext, pool, userID)
}
