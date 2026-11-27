package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	financialHistoryMaxPageSize = 200
	historyTimestampLayout      = "2006-01-02T15:04:05.000Z"
)

type financialHistoryCursor struct {
	createdAt time.Time
	id        string
}

type financialHistoryCursorState struct {
	cursor *financialHistoryCursor
	done   bool
}

type financialHistoryOptions struct {
	limit         int
	snapshot      string
	snapshotTime  time.Time
	expenseCursor financialHistoryCursorState
	incomeCursor  financialHistoryCursorState
}

type financialHistoryPage struct {
	Snapshot          string            `json:"snapshot"`
	NextExpenseCursor *string           `json:"nextExpenseCursor"`
	NextIncomeCursor  *string           `json:"nextIncomeCursor"`
	Expenses          []expense         `json:"expenses"`
	Incomes           []json.RawMessage `json:"incomes"`
}

type financialHistoryPageReader interface {
	ListFinancialHistoryPage(context.Context, string, financialHistoryOptions) (financialHistoryPage, error)
}

type postgresFinancialHistoryRepository struct {
	database *postgresDatabase
}

func financialHistoryHandler(
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
	pageReader financialHistoryPageReader,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			writeFinancialHistoryInputError(w, r, errors.New("Invalid financial history query"))
			return
		}
		options, err := parseFinancialHistoryOptions(query, time.Now())
		if err != nil {
			writeFinancialHistoryInputError(w, r, err)
			return
		}
		if pageReader == nil {
			writeInternalError(w, r)
			return
		}
		page, err := pageReader.ListFinancialHistoryPage(r.Context(), userID, options)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		page.Snapshot = options.snapshot
		if page.Expenses == nil {
			page.Expenses = []expense{}
		}
		if page.Incomes == nil {
			page.Incomes = []json.RawMessage{}
		}
		writeJSON(w, r, http.StatusOK, page)
	}
}

func writeFinancialHistoryInputError(w http.ResponseWriter, r *http.Request, err error) {
	writeJSON(w, r, http.StatusBadRequest, struct {
		Error string `json:"error"`
	}{err.Error()})
}

func parseFinancialHistoryOptions(query url.Values, now time.Time) (financialHistoryOptions, error) {
	allowed := map[string]bool{
		"limit": true, "snapshot": true, "expenseCursor": true, "incomeCursor": true,
	}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 || values[0] == "" {
			return financialHistoryOptions{}, errors.New("Invalid financial history query")
		}
	}

	limit := financialHistoryMaxPageSize
	if values, ok := query["limit"]; ok {
		if !validFinancialHistoryLimit(values[0]) {
			return financialHistoryOptions{}, errors.New("Invalid financial history page limit")
		}
		parsed, err := strconv.Atoi(values[0])
		if err != nil || parsed > financialHistoryMaxPageSize {
			return financialHistoryOptions{}, errors.New("Invalid financial history page limit")
		}
		limit = parsed
	}

	now = now.UTC().Truncate(time.Millisecond)
	snapshot := now.Format(historyTimestampLayout)
	snapshotTime := now
	_, hasSnapshot := query["snapshot"]
	if values, ok := query["snapshot"]; ok {
		parsed, err := parseFinancialHistoryTimestamp(values[0])
		if err != nil || parsed.UnixMilli() < 0 || parsed.After(now.Add(time.Minute)) {
			return financialHistoryOptions{}, errors.New("Invalid financial history snapshot")
		}
		snapshot = values[0]
		snapshotTime = parsed
	}
	if !hasSnapshot && (query.Has("expenseCursor") || query.Has("incomeCursor")) {
		return financialHistoryOptions{}, errors.New("Invalid financial history snapshot")
	}

	options := financialHistoryOptions{
		limit:        limit,
		snapshot:     snapshot,
		snapshotTime: snapshotTime,
	}
	if values, ok := query["expenseCursor"]; ok {
		cursor, err := parseFinancialHistoryCursor(values[0])
		if err != nil {
			return financialHistoryOptions{}, err
		}
		options.expenseCursor = cursor
	}
	if values, ok := query["incomeCursor"]; ok {
		cursor, err := parseFinancialHistoryCursor(values[0])
		if err != nil {
			return financialHistoryOptions{}, err
		}
		options.incomeCursor = cursor
	}
	for _, cursor := range []financialHistoryCursorState{options.expenseCursor, options.incomeCursor} {
		if cursor.cursor != nil && cursor.cursor.createdAt.After(snapshotTime) {
			return financialHistoryOptions{}, errors.New("Invalid financial history cursor snapshot")
		}
	}
	return options, nil
}

func validFinancialHistoryLimit(value string) bool {
	if len(value) == 0 || len(value) > 3 || value[0] < '1' || value[0] > '9' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func parseFinancialHistoryTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(historyTimestampLayout, value)
	if err != nil || parsed.UTC().Format(historyTimestampLayout) != value {
		return time.Time{}, errors.New("invalid timestamp")
	}
	return parsed.UTC(), nil
}

func parseFinancialHistoryCursor(value string) (financialHistoryCursorState, error) {
	if value == "end" {
		return financialHistoryCursorState{done: true}, nil
	}
	if len(value) > 512 || value == "" {
		return financialHistoryCursorState{}, errors.New("Invalid financial history cursor")
	}
	for _, character := range value {
		if !(character >= 'A' && character <= 'Z') && !(character >= 'a' && character <= 'z') && !(character >= '0' && character <= '9') && character != '_' && character != '-' {
			return financialHistoryCursorState{}, errors.New("Invalid financial history cursor")
		}
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil {
		return financialHistoryCursorState{}, errors.New("Invalid financial history cursor")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(decoded, &payload); err != nil || payload == nil {
		return financialHistoryCursorState{}, errors.New("Invalid financial history cursor")
	}
	var id, rawCreatedAt string
	if json.Unmarshal(payload["id"], &id) != nil || id == "" || len(id) > 128 || json.Unmarshal(payload["createdAt"], &rawCreatedAt) != nil {
		return financialHistoryCursorState{}, errors.New("Invalid financial history cursor")
	}
	createdAt, err := parseFinancialHistoryTimestamp(rawCreatedAt)
	if err != nil {
		return financialHistoryCursorState{}, errors.New("Invalid financial history cursor")
	}
	return financialHistoryCursorState{cursor: &financialHistoryCursor{createdAt: createdAt, id: id}}, nil
}

func encodeFinancialHistoryCursor(cursor financialHistoryCursor) (string, error) {
	if cursor.id == "" {
		return "", errors.New("empty financial history cursor id")
	}
	value, err := json.Marshal(struct {
		CreatedAt string `json:"createdAt"`
		ID        string `json:"id"`
	}{cursor.createdAt.UTC().Format(historyTimestampLayout), cursor.id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (repository postgresFinancialHistoryRepository) ListFinancialHistoryPage(
	ctx context.Context,
	userID string,
	options financialHistoryOptions,
) (financialHistoryPage, error) {
	if repository.database == nil {
		return financialHistoryPage{}, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return financialHistoryPage{}, err
	}
	return queryFinancialHistoryPage(queryContext, pool, userID, options)
}

type financialHistoryQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func queryFinancialHistoryPage(
	ctx context.Context,
	queryer financialHistoryQueryer,
	userID string,
	options financialHistoryOptions,
) (financialHistoryPage, error) {
	if options.limit < 1 || options.limit > financialHistoryMaxPageSize {
		return financialHistoryPage{}, errors.New("Invalid financial history page limit")
	}
	page := financialHistoryPage{
		Snapshot: options.snapshot,
		Expenses: []expense{},
		Incomes:  []json.RawMessage{},
	}
	if !options.expenseCursor.done {
		cursorTime, cursorID := financialHistoryCursorQueryValues(options.expenseCursor)
		expenses, err := queryExpensesWithQuery(
			ctx,
			queryer,
			financialHistoryExpenseQuery,
			userID,
			options.snapshotTime,
			cursorTime,
			cursorID,
			options.limit+1,
		)
		if err != nil {
			return financialHistoryPage{}, err
		}
		if len(expenses) > options.limit {
			last := expenses[options.limit-1]
			createdAt, err := parseFinancialHistoryTimestamp(last.CreatedAt)
			if err != nil {
				return financialHistoryPage{}, err
			}
			cursor, err := encodeFinancialHistoryCursor(financialHistoryCursor{createdAt: createdAt, id: last.ID})
			if err != nil {
				return financialHistoryPage{}, err
			}
			page.NextExpenseCursor = &cursor
			expenses = expenses[:options.limit]
		}
		page.Expenses = expenses
	}
	if !options.incomeCursor.done {
		cursorTime, cursorID := financialHistoryCursorQueryValues(options.incomeCursor)
		incomes, err := queryEntryProjections(
			ctx,
			queryer,
			financialHistoryIncomeQuery,
			userID,
			options.snapshotTime,
			cursorTime,
			cursorID,
			options.limit+1,
		)
		if err != nil {
			return financialHistoryPage{}, err
		}
		if len(incomes) > options.limit {
			cursor, err := financialHistoryCursorFromProjection(incomes[options.limit-1])
			if err != nil {
				return financialHistoryPage{}, err
			}
			next, err := encodeFinancialHistoryCursor(cursor)
			if err != nil {
				return financialHistoryPage{}, err
			}
			page.NextIncomeCursor = &next
			incomes = incomes[:options.limit]
		}
		page.Incomes = incomes
	}
	return page, nil
}

func financialHistoryCursorQueryValues(state financialHistoryCursorState) (any, string) {
	if state.cursor == nil {
		return nil, ""
	}
	return state.cursor.createdAt, state.cursor.id
}

func financialHistoryCursorFromProjection(projection json.RawMessage) (financialHistoryCursor, error) {
	var row struct {
		ID        string `json:"id"`
		CreatedAt string `json:"createdAt"`
	}
	if err := json.Unmarshal(projection, &row); err != nil || row.ID == "" {
		return financialHistoryCursor{}, errors.New("invalid financial history projection cursor")
	}
	createdAt, err := parseFinancialHistoryTimestamp(row.CreatedAt)
	if err != nil {
		return financialHistoryCursor{}, err
	}
	return financialHistoryCursor{createdAt: createdAt, id: row.ID}, nil
}

const financialHistoryExpenseQuery = expenseProjectionBaseSQL + `
	WHERE e."userId" = $1
		AND e."systemOrigin" = 'manual'
		AND e."voidedAt" IS NULL
		AND e."createdAt" <= $2::timestamp
		AND (
			$3::timestamp IS NULL
			OR e."createdAt" < $3::timestamp
			OR (e."createdAt" = $3::timestamp AND e."id" < $4)
		)
	ORDER BY e."createdAt" DESC, e."id" DESC
	LIMIT $5`

const financialHistoryIncomeQuery = entryProjectionBaseSQL + `
	WHERE entry."userId" = $1
		AND entry."systemOrigin" = 'manual'
		AND entry."createdAt" <= $2::timestamp
		AND (
			$3::timestamp IS NULL
			OR entry."createdAt" < $3::timestamp
			OR (entry."createdAt" = $3::timestamp AND entry."id" < $4)
		)
	ORDER BY entry."createdAt" DESC, entry."id" DESC
	LIMIT $5`
