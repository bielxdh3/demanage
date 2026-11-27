package main

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// Prisma DateTime uses TIMESTAMP(3); format it as Date.toJSON's UTC millisecond string.
const entryProjectionBaseSQL = `
	SELECT jsonb_build_object(
		'id', entry.id,
		'userId', entry."userId",
		'customTagId', entry."customTagId",
		'name', entry.name,
		'notes', entry.notes,
		'amount', entry.amount,
		'type', entry.type,
		'frequency', entry.frequency,
		'receiveDay', entry."receiveDay",
		'startsAt', to_char(entry."startsAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		'endsAt', to_char(entry."endsAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		'date', to_char(entry.date, 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		'receiptHoldForMonth', entry."receiptHoldForMonth",
		'receivedForMonth', entry."receivedForMonth",
		'receivedAt', to_char(entry."receivedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		'archivedAt', to_char(entry."archivedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		'systemOrigin', entry."systemOrigin",
		'createdAt', to_char(entry."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		'updatedAt', to_char(entry."updatedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		'customTag', CASE
			WHEN custom_tag.id IS NULL THEN NULL
			ELSE jsonb_build_object(
				'id', custom_tag.id,
				'name', custom_tag.name,
				'color', custom_tag.color
			)
		END,
		'salaryHolds', COALESCE((
			SELECT jsonb_agg(jsonb_build_object(
				'id', hold.id,
				'entryId', hold."entryId",
				'month', hold.month,
				'createdAt', to_char(hold."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
			) ORDER BY hold.month ASC, hold.id ASC)
			FROM "SalaryHold" AS hold
			WHERE hold."entryId" = entry.id
		), '[]'::jsonb),
		'salaryVersions', COALESCE((
			SELECT jsonb_agg(jsonb_build_object(
				'id', version.id,
				'entryId', version."entryId",
				'month', version.month,
				'amount', version.amount,
				'receiveDay', version."receiveDay",
				'createdAt', to_char(version."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
			) ORDER BY version.month ASC, version.id ASC)
			FROM "SalaryVersion" AS version
			WHERE version."entryId" = entry.id
		), '[]'::jsonb),
		'receipts', COALESCE((
			SELECT jsonb_agg(jsonb_build_object(
				'id', receipt.id,
				'entryId', receipt."entryId",
				'month', receipt.month,
				'amount', receipt.amount,
				'receivedAt', to_char(receipt."receivedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
				'createdAt', to_char(receipt."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
			) ORDER BY receipt."receivedAt" ASC, receipt."createdAt" ASC, receipt.id ASC)
			FROM "EntryReceipt" AS receipt
			WHERE receipt."entryId" = entry.id
		), '[]'::jsonb)
	) AS entry
	FROM "Entry" AS entry
	LEFT JOIN "CustomTag" AS custom_tag
		ON custom_tag.id = entry."customTagId"
		AND custom_tag."userId" = entry."userId"
		AND custom_tag.scope = 'income'
	`

const entryProjectionSQL = entryProjectionBaseSQL + `
	WHERE entry."userId" = $1
		AND entry."archivedAt" IS NULL
		AND entry."systemOrigin" = 'manual'
	ORDER BY entry."createdAt" DESC, entry.id DESC`

type entryProjectionRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}

type entryProjectionQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func listEntries(ctx context.Context, queryer entryProjectionQueryer, userID string) ([]json.RawMessage, error) {
	return queryEntryProjections(ctx, queryer, entryProjectionSQL, userID)
}

func queryEntryProjections(ctx context.Context, queryer entryProjectionQueryer, query string, args ...any) ([]json.RawMessage, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return collectEntryProjections(rows)
}

func collectEntryProjections(rows entryProjectionRows) ([]json.RawMessage, error) {
	defer rows.Close()
	entries := make([]json.RawMessage, 0)
	for rows.Next() {
		var projection []byte
		if err := rows.Scan(&projection); err != nil {
			return nil, err
		}
		entries = append(entries, append(json.RawMessage(nil), projection...))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
