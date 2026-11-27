package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const piggyCashMovementsProjectionSQL = `
	SELECT jsonb_build_object(
		'type', movement.type,
		'amount', movement.amount,
		'date', to_char(movement.date, 'YYYY-MM-DD')
	) AS movement
	FROM "PiggyTransaction" AS movement
	WHERE movement."userId" = $1
		AND (
			(movement.type = 'deposit' AND movement."expenseId" IS NOT NULL)
			OR (movement.type = 'withdraw' AND movement."entryId" IS NOT NULL)
		)
	ORDER BY movement.date ASC, movement."createdAt" ASC`

const piggyBankTransactionsExistenceSQL = `
	SELECT EXISTS (
		SELECT 1
		FROM "PiggyBank" AS bank
		WHERE bank.id = $1 AND bank."userId" = $2
	)`

const piggyBankTransactionsProjectionSQL = `
	SELECT jsonb_build_object(
		'id', movement.id,
		'piggyBankId', movement."piggyBankId",
		'type', movement.type,
		'source', movement.source,
		'amount', movement.amount,
		'date', to_char(movement.date, 'YYYY-MM-DD'),
		'expenseId', movement."expenseId",
		'entryId', movement."entryId",
		'note', movement.note,
		'cdiRate', movement."cdiRate",
		'cdiPercent', movement."cdiPercent",
		'baseBalance', CASE
			WHEN movement."baseBalance" IS NULL THEN NULL
			ELSE round(movement."baseBalance", 2)
		END,
		'resultingBalance', CASE
			WHEN movement."resultingBalance" IS NULL THEN NULL
			ELSE round(movement."resultingBalance", 2)
		END,
		'createdAt', to_char(movement."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
	) AS movement
	FROM "PiggyTransaction" AS movement
	WHERE movement."piggyBankId" = $1 AND movement."userId" = $2
	ORDER BY movement.date DESC, movement."createdAt" DESC`

type piggyReadQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type piggyProjectionRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}

type piggyBankReadRepository interface {
	ListPiggyCashMovements(context.Context, string) ([]json.RawMessage, error)
	ListPiggyBankTransactions(context.Context, string, string) ([]json.RawMessage, bool, error)
}

type postgresPiggyBankReadRepository struct {
	database *postgresDatabase
}

func (repository postgresPiggyBankReadRepository) ListPiggyCashMovements(ctx context.Context, userID string) ([]json.RawMessage, error) {
	if repository.database == nil {
		return nil, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, err
	}
	return listPiggyCashMovements(queryContext, pool, userID)
}

func (repository postgresPiggyBankReadRepository) ListPiggyBankTransactions(ctx context.Context, userID, bankID string) ([]json.RawMessage, bool, error) {
	if repository.database == nil {
		return nil, false, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, false, err
	}
	return listPiggyBankTransactions(queryContext, pool, userID, bankID)
}

func listPiggyCashMovements(ctx context.Context, queryer piggyReadQueryer, userID string) ([]json.RawMessage, error) {
	rows, err := queryer.Query(ctx, piggyCashMovementsProjectionSQL, userID)
	if err != nil {
		return nil, err
	}
	return collectPiggyProjections(rows)
}

func listPiggyBankTransactions(ctx context.Context, queryer piggyReadQueryer, userID, bankID string) ([]json.RawMessage, bool, error) {
	var bankExists bool
	if err := queryer.QueryRow(ctx, piggyBankTransactionsExistenceSQL, bankID, userID).Scan(&bankExists); err != nil {
		return nil, false, err
	}
	if !bankExists {
		return nil, false, nil
	}
	rows, err := queryer.Query(ctx, piggyBankTransactionsProjectionSQL, bankID, userID)
	if err != nil {
		return nil, true, err
	}
	transactions, err := collectPiggyProjections(rows)
	return transactions, true, err
}

func collectPiggyProjections(rows piggyProjectionRows) ([]json.RawMessage, error) {
	defer rows.Close()
	projections := make([]json.RawMessage, 0)
	for rows.Next() {
		var projection []byte
		if err := rows.Scan(&projection); err != nil {
			return nil, err
		}
		projections = append(projections, append(json.RawMessage(nil), projection...))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projections, nil
}

func newPiggyBankReadProjectionHandler(
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
	repository piggyBankReadRepository,
) http.Handler {
	mux := http.NewServeMux()
	registerPiggyBankReadProjections(mux, jwtKey, sessionReader, repository)
	return mux
}

func registerPiggyBankReadProjections(
	mux *http.ServeMux,
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
	repository piggyBankReadRepository,
) {
	cashMovementsHandler := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		if repository == nil {
			writeInternalError(w, r)
			return
		}
		movements, err := repository.ListPiggyCashMovements(r.Context(), userID)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if movements == nil {
			movements = []json.RawMessage{}
		}
		writeJSON(w, r, http.StatusOK, movements)
	}
	mux.HandleFunc("GET /piggy-banks/cash-movements", cashMovementsHandler)
	mux.HandleFunc("GET /piggy-banks/cash-movements/{$}", cashMovementsHandler)

	transactionsHandler := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		if repository == nil {
			writeInternalError(w, r)
			return
		}
		transactions, bankExists, err := repository.ListPiggyBankTransactions(r.Context(), userID, r.PathValue("id"))
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if !bankExists {
			writeJSON(w, r, http.StatusNotFound, struct {
				Error string `json:"error"`
			}{"Cofre não encontrado"})
			return
		}
		if transactions == nil {
			transactions = []json.RawMessage{}
		}
		writeJSON(w, r, http.StatusOK, transactions)
	}
	mux.HandleFunc("GET /piggy-banks/{id}/transactions", transactionsHandler)
	mux.HandleFunc("GET /piggy-banks/{id}/transactions/{$}", transactionsHandler)
}

var _ piggyReadQueryer = (*pgxpool.Pool)(nil)
