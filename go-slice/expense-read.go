package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type expenseQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type postgresExpenseRepository struct {
	database *postgresDatabase
}

func (repository postgresExpenseRepository) ListExpenses(ctx context.Context, userID string) ([]expense, error) {
	if repository.database == nil {
		return nil, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, err
	}
	return queryExpenses(queryContext, pool, userID)
}

func queryExpenses(ctx context.Context, queryer expenseQueryer, userID string) ([]expense, error) {
	rows, err := queryer.Query(ctx, expenseListQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	expenses := make([]expense, 0)
	for rows.Next() {
		item, err := scanExpense(rows)
		if err != nil {
			return nil, err
		}
		expenses = append(expenses, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return expenses, nil
}

func scanExpense(row pgx.Row) (expense, error) {
	var item expense
	var amount string
	var dueDay pgtype.Int4
	var cardID, customTagID, startsAt, endsAt, occurredAt pgtype.Text
	var paidForMonth, paidAt, notes pgtype.Text
	var billingPeriodStart, billingPeriodEnd, archivedAt, voidedAt pgtype.Text
	var customTagJSON, splitsJSON, paymentsJSON []byte
	if err := row.Scan(
		&item.ID,
		&item.UserID,
		&cardID,
		&customTagID,
		&item.Name,
		&amount,
		&item.Category,
		&item.Frequency,
		&dueDay,
		&startsAt,
		&endsAt,
		&occurredAt,
		&paidForMonth,
		&paidAt,
		&notes,
		&item.IsInvoice,
		&billingPeriodStart,
		&billingPeriodEnd,
		&archivedAt,
		&voidedAt,
		&item.SystemOrigin,
		&item.CreatedAt,
		&item.UpdatedAt,
		&customTagJSON,
		&splitsJSON,
		&paymentsJSON,
	); err != nil {
		return expense{}, err
	}
	parsedAmount, err := parseMoney(amount)
	if err != nil {
		return expense{}, err
	}
	item.Amount = parsedAmount
	item.CardID = textPointer(cardID)
	item.CustomTagID = textPointer(customTagID)
	if dueDay.Valid {
		value := dueDay.Int32
		item.DueDay = &value
	}
	item.StartsAt = textPointer(startsAt)
	item.EndsAt = textPointer(endsAt)
	item.OccurredAt = textPointer(occurredAt)
	item.PaidForMonth = textPointer(paidForMonth)
	item.PaidAt = textPointer(paidAt)
	item.Notes = textPointer(notes)
	item.BillingPeriodStart = textPointer(billingPeriodStart)
	item.BillingPeriodEnd = textPointer(billingPeriodEnd)
	item.ArchivedAt = textPointer(archivedAt)
	item.VoidedAt = textPointer(voidedAt)
	if err := json.Unmarshal(customTagJSON, &item.CustomTag); err != nil {
		return expense{}, err
	}
	if err := json.Unmarshal(splitsJSON, &item.Splits); err != nil {
		return expense{}, err
	}
	if item.Splits == nil {
		item.Splits = []expenseSplit{}
	}
	if err := json.Unmarshal(paymentsJSON, &item.Payments); err != nil {
		return expense{}, err
	}
	if item.Payments == nil {
		item.Payments = []expensePayment{}
	}
	return item, nil
}

func parseMoney(value string) (money, error) {
	parsed, err := parseScaled([]byte(value), 2)
	return money(parsed), err
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

const expenseListQuery = `
	SELECT
		e."id",
		e."userId",
		e."cardId",
		e."customTagId",
		e."name",
		e."amount"::text,
		e."category",
		e."frequency",
		e."dueDay",
		to_char(e."startsAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."endsAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."occurredAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		e."paidForMonth",
		to_char(e."paidAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		e."notes",
		e."isInvoice",
		to_char(e."billingPeriodStart", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."billingPeriodEnd", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."archivedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."voidedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		e."systemOrigin",
		to_char(e."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."updatedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		CASE WHEN tag."id" IS NULL THEN 'null'::json
			ELSE json_build_object('id', tag."id", 'name', tag."name", 'color', tag."color")
		END AS "customTag",
		COALESCE((
			SELECT json_agg(json_build_object(
				'id', split."id",
				'kind', split."kind",
				'cardId', split."cardId",
				'percent', split."percent"::text,
				'amount', split."amount"::text,
				'cardName', card."name"
			) ORDER BY split."kind" ASC, split."percent" DESC)
			FROM "ExpenseSplit" AS split
			LEFT JOIN "Card" AS card
				ON card."id" = split."cardId" AND card."userId" = e."userId"
			WHERE split."expenseId" = e."id"
		), '[]'::json) AS "splits",
		COALESCE((
			SELECT json_agg(json_build_object(
				'id', payment."id",
				'expenseId', payment."expenseId",
				'month', payment."month",
				'amount', payment."amount"::text,
				'paidAt', to_char(payment."paidAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
				'createdAt', to_char(payment."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
			) ORDER BY payment."paidAt" ASC, payment."createdAt" ASC)
			FROM "ExpensePayment" AS payment
			WHERE payment."expenseId" = e."id"
		), '[]'::json) AS "payments"
	FROM "Expense" AS e
	LEFT JOIN "CustomTag" AS tag
		ON tag."id" = e."customTagId" AND tag."userId" = e."userId" AND tag."scope" = 'expense'
	WHERE e."userId" = $1
		AND e."archivedAt" IS NULL
		AND e."systemOrigin" = 'manual'
	ORDER BY e."createdAt" DESC`
