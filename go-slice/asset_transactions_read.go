package main

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type assetTransactionQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type postgresAssetTransactionRepository struct {
	database *postgresDatabase
}

func (repository postgresAssetTransactionRepository) ListAssetTransactions(ctx context.Context, userID, asset string) ([]assetTransaction, error) {
	if repository.database == nil {
		return nil, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, err
	}
	return listAssetTransactions(queryContext, pool, userID, asset)
}

func listAssetTransactions(ctx context.Context, queryer assetTransactionQueryer, userID, asset string) ([]assetTransaction, error) {
	rows, err := queryer.Query(ctx, assetTransactionListQuery, userID, asset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transactions := make([]assetTransaction, 0)
	for rows.Next() {
		transaction, err := scanAssetTransaction(rows)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return transactions, nil
}

func scanAssetTransaction(row pgx.Row) (assetTransaction, error) {
	var transaction assetTransaction
	var feePercent, note, expenseID, entryID pgtype.Text
	if err := row.Scan(
		&transaction.ID,
		&transaction.Asset,
		&transaction.Type,
		&transaction.Quantity,
		&transaction.CashAmountBRL,
		&transaction.FeeAmountBRL,
		&feePercent,
		&transaction.CostBasisKnown,
		&transaction.Date,
		&note,
		&expenseID,
		&entryID,
		&transaction.CreatedAt,
	); err != nil {
		return assetTransaction{}, err
	}
	transaction.Quantity = plainAssetDecimal(transaction.Quantity)
	transaction.CashAmountBRL = plainAssetDecimal(transaction.CashAmountBRL)
	transaction.FeeAmountBRL = plainAssetDecimal(transaction.FeeAmountBRL)
	transaction.FeePercent = textPointer(feePercent)
	if transaction.FeePercent != nil {
		plain := plainAssetDecimal(*transaction.FeePercent)
		transaction.FeePercent = &plain
	}
	transaction.Note = textPointer(note)
	transaction.ExpenseID = textPointer(expenseID)
	transaction.EntryID = textPointer(entryID)
	return transaction, nil
}

const assetTransactionListQuery = `
	SELECT
		"id",
		"asset",
		"type",
		"quantity"::text,
		"cashAmountBrl"::text,
		"feeAmountBrl"::text,
		"feePercent"::text,
		"costBasisKnown",
		to_char("date", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		"note",
		"expenseId",
		"entryId",
		to_char("createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
	FROM "AssetTransaction"
	WHERE "userId" = $1 AND "asset" = $2
	ORDER BY "date" DESC, "createdAt" DESC`
