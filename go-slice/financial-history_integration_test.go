//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresFinancialHistoryProjectionIntegration(t *testing.T) {
	database := newPostgresDatabase(requireGitHubPostgresDSN(t))
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.pool()
	if err != nil {
		t.Fatalf("create PostgreSQL pool: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin isolated projection transaction: %v", err)
	}
	rollbackPending := true
	defer func() {
		if rollbackPending {
			if err := tx.Rollback(context.Background()); err != nil {
				t.Errorf("rollback isolated projection transaction: %v", err)
			}
		}
	}()
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO pg_temp`); err != nil {
		t.Fatalf("restrict transaction search path to temporary tables: %v", err)
	}

	fixture, err := os.ReadFile("testdata/financial-history.integration.sql")
	if err != nil {
		t.Fatalf("read financial-history projection fixture: %v", err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("apply transaction-local financial-history fixture: %v", err)
		}
	}

	const snapshot = "2026-11-10T00:00:00.000Z"
	now := time.Date(2026, time.November, 27, 12, 0, 0, 0, time.UTC)
	options, err := parseFinancialHistoryOptions(url.Values{"snapshot": {snapshot}, "limit": {"2"}}, now)
	if err != nil {
		t.Fatalf("parse initial financial-history page: %v", err)
	}
	firstQueries := &recordingFinancialHistoryQueryer{queryer: tx}
	firstPage, err := queryFinancialHistoryPage(ctx, firstQueries, "synthetic-owner-a", options)
	if err != nil {
		t.Fatalf("query initial financial-history page: %v", err)
	}
	if len(firstQueries.calls) != 2 {
		t.Fatalf("initial page issued %d queries, want one per ledger", len(firstQueries.calls))
	}
	for _, call := range firstQueries.calls {
		if !strings.Contains(strings.ToLower(call.query), "limit $5") || len(call.args) != 5 || call.args[0] != "synthetic-owner-a" || call.args[4] != 3 {
			t.Fatalf("bounded owner-scoped query = %q with args %#v", call.query, call.args)
		}
	}
	if firstPage.Snapshot != snapshot || firstPage.NextExpenseCursor == nil || firstPage.NextIncomeCursor == nil {
		t.Fatalf("first page snapshot/cursors = %q / %v / %v", firstPage.Snapshot, firstPage.NextExpenseCursor, firstPage.NextIncomeCursor)
	}
	if got, want := financialHistoryExpenseIDs(firstPage.Expenses), []string{"expense-z-archived", "expense-m-tie"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first expense page order = %#v, want %#v", got, want)
	}
	firstIncomes := decodeEntryProjectionRows(t, firstPage.Incomes)
	if got, want := entryProjectionIDs(t, firstIncomes), []string{"entry-z-archived", "entry-m-tie"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first income page order = %#v, want %#v", got, want)
	}

	archivedExpense := firstPage.Expenses[0]
	if archivedExpense.UserID != "synthetic-owner-a" || archivedExpense.ArchivedAt == nil || archivedExpense.VoidedAt != nil {
		t.Fatalf("archived manual expense projection = %#v", archivedExpense)
	}
	if archivedExpense.Amount != money(999999999999) || archivedExpense.CustomTag != nil || len(archivedExpense.Splits) != 3 || len(archivedExpense.Payments) != 2 {
		t.Fatalf("archived expense amount/relations = %#v", archivedExpense)
	}
	if archivedExpense.Splits[0].CardName == nil || *archivedExpense.Splits[0].CardName != "Owner card" || archivedExpense.Splits[1].CardName != nil || archivedExpense.Splits[2].CardName != nil {
		t.Fatalf("expense split card ownership = %#v", archivedExpense.Splits)
	}
	if archivedExpense.Splits[0].Amount != money(600) || archivedExpense.Splits[0].Percent != percentage(6000) || archivedExpense.Payments[0].ID != "payment-first" || archivedExpense.Payments[0].Amount != money(10) {
		t.Fatalf("expense exact BRL values or relation order = %#v", archivedExpense)
	}
	expenseJSON, err := json.Marshal(firstPage.Expenses)
	if err != nil {
		t.Fatalf("marshal expense projection: %v", err)
	}
	expenseRows := decodeFinancialHistoryExpenseRows(t, expenseJSON)
	assertEntryJSONNumber(t, expenseRows[0]["amount"], "9999999999.99")
	if payments := entryProjectionArray(t, expenseRows[0]["payments"]); len(payments) != 2 {
		t.Fatalf("expense payment JSON = %#v", payments)
	} else {
		assertEntryJSONNumber(t, entryProjectionObject(t, payments[0])["amount"], "0.1")
	}
	if len(firstPage.Expenses[1].Splits) != 0 || firstPage.Expenses[1].Splits == nil || len(firstPage.Expenses[1].Payments) != 0 || firstPage.Expenses[1].Payments == nil || firstPage.Expenses[1].CustomTag != nil {
		t.Fatalf("empty expense relations/null tag = %#v", firstPage.Expenses[1])
	}

	archivedIncome := firstIncomes[0]
	if archivedIncome["userId"] != "synthetic-owner-a" || archivedIncome["archivedAt"] != "2026-11-08T00:00:00.000Z" {
		t.Fatalf("archived entry owner/timestamp = %#v", archivedIncome)
	}
	assertEntryJSONNumber(t, archivedIncome["amount"], "9999999999.99")
	assertEntryJSONNull(t, archivedIncome, "customTag")
	holds := entryProjectionArray(t, archivedIncome["salaryHolds"])
	versions := entryProjectionArray(t, archivedIncome["salaryVersions"])
	receipts := entryProjectionArray(t, archivedIncome["receipts"])
	if len(holds) != 2 || len(versions) != 2 || len(receipts) != 2 {
		t.Fatalf("entry nested history arrays = holds %#v versions %#v receipts %#v", holds, versions, receipts)
	}
	assertEntryJSONNumber(t, entryProjectionObject(t, versions[0])["amount"], "1234567890.12")
	assertEntryJSONNumber(t, entryProjectionObject(t, receipts[0])["amount"], "0.1")
	if len(entryProjectionArray(t, firstIncomes[1]["salaryHolds"])) != 0 || len(entryProjectionArray(t, firstIncomes[1]["salaryVersions"])) != 0 || len(entryProjectionArray(t, firstIncomes[1]["receipts"])) != 0 {
		t.Fatalf("empty entry relation arrays = %#v", firstIncomes[1])
	}
	assertEntryJSONNull(t, firstIncomes[1], "customTag")

	pageTwoOptions, err := parseFinancialHistoryOptions(url.Values{
		"snapshot":      {snapshot},
		"limit":         {"2"},
		"expenseCursor": {*firstPage.NextExpenseCursor},
		"incomeCursor":  {*firstPage.NextIncomeCursor},
	}, now)
	if err != nil {
		t.Fatalf("parse next financial-history page: %v", err)
	}
	secondPage, err := queryFinancialHistoryPage(ctx, tx, "synthetic-owner-a", pageTwoOptions)
	if err != nil {
		t.Fatalf("query next financial-history page: %v", err)
	}
	if secondPage.NextExpenseCursor != nil || secondPage.NextIncomeCursor != nil {
		t.Fatalf("final page cursors = %v / %v, want null", secondPage.NextExpenseCursor, secondPage.NextIncomeCursor)
	}
	if got, want := financialHistoryExpenseIDs(secondPage.Expenses), []string{"expense-a-tie", "expense-older"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second expense page = %#v, want %#v", got, want)
	}
	if secondPage.Expenses[0].CustomTag == nil || secondPage.Expenses[0].CustomTag.ID != "tag-expense-a" {
		t.Fatalf("valid expense custom tag = %#v", secondPage.Expenses[0].CustomTag)
	}
	secondIncomes := decodeEntryProjectionRows(t, secondPage.Incomes)
	if got, want := entryProjectionIDs(t, secondIncomes), []string{"entry-a-tie", "entry-older"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second income page = %#v, want %#v", got, want)
	}
	if customTag := entryProjectionObject(t, secondIncomes[0]["customTag"]); customTag["id"] != "tag-income-a" {
		t.Fatalf("valid income custom tag = %#v", customTag)
	}

	expenseEndedOptions, err := parseFinancialHistoryOptions(url.Values{
		"snapshot":      {snapshot},
		"limit":         {"2"},
		"expenseCursor": {"end"},
		"incomeCursor":  {*firstPage.NextIncomeCursor},
	}, now)
	if err != nil {
		t.Fatalf("parse income-only continuation: %v", err)
	}
	incomeOnlyQueries := &recordingFinancialHistoryQueryer{queryer: tx}
	incomeOnlyPage, err := queryFinancialHistoryPage(ctx, incomeOnlyQueries, "synthetic-owner-a", expenseEndedOptions)
	if err != nil {
		t.Fatalf("query income-only continuation: %v", err)
	}
	if len(incomeOnlyPage.Expenses) != 0 || len(incomeOnlyPage.Incomes) != 2 {
		t.Fatalf("expense end sentinel affected independent income cursor: expenses %#v incomes %#v", incomeOnlyPage.Expenses, incomeOnlyPage.Incomes)
	}
	if len(incomeOnlyQueries.calls) != 1 || len(incomeOnlyQueries.calls[0].args) != 5 || incomeOnlyQueries.calls[0].args[0] != "synthetic-owner-a" || incomeOnlyQueries.calls[0].args[4] != 3 {
		t.Fatalf("income-only continuation queries = %#v, want one owner-scoped limit+1 query", incomeOnlyQueries.calls)
	}

	incomeEndedOptions, err := parseFinancialHistoryOptions(url.Values{
		"snapshot":      {snapshot},
		"limit":         {"2"},
		"expenseCursor": {*firstPage.NextExpenseCursor},
		"incomeCursor":  {"end"},
	}, now)
	if err != nil {
		t.Fatalf("parse expense-only continuation: %v", err)
	}
	expenseOnlyQueries := &recordingFinancialHistoryQueryer{queryer: tx}
	expenseOnlyPage, err := queryFinancialHistoryPage(ctx, expenseOnlyQueries, "synthetic-owner-a", incomeEndedOptions)
	if err != nil {
		t.Fatalf("query expense-only continuation: %v", err)
	}
	if len(expenseOnlyPage.Expenses) != 2 || len(expenseOnlyPage.Incomes) != 0 {
		t.Fatalf("income end sentinel affected independent expense cursor: expenses %#v incomes %#v", expenseOnlyPage.Expenses, expenseOnlyPage.Incomes)
	}
	if len(expenseOnlyQueries.calls) != 1 || len(expenseOnlyQueries.calls[0].args) != 5 || expenseOnlyQueries.calls[0].args[0] != "synthetic-owner-a" || expenseOnlyQueries.calls[0].args[4] != 3 {
		t.Fatalf("expense-only continuation queries = %#v, want one owner-scoped limit+1 query", expenseOnlyQueries.calls)
	}

	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback isolated projection transaction: %v", err)
	}
	rollbackPending = false
}

func financialHistoryExpenseIDs(expenses []expense) []string {
	ids := make([]string, 0, len(expenses))
	for _, item := range expenses {
		ids = append(ids, item.ID)
	}
	return ids
}

func entryProjectionIDs(t *testing.T, entries []map[string]any) []string {
	t.Helper()
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entryProjectionString(t, entry["id"]))
	}
	return ids
}

func decodeFinancialHistoryExpenseRows(t *testing.T, encoded []byte) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	var expenses []map[string]any
	if err := decoder.Decode(&expenses); err != nil {
		t.Fatalf("decode expense projection rows: %v", err)
	}
	return expenses
}

type recordedFinancialHistoryQuery struct {
	query string
	args  []any
}

type recordingFinancialHistoryQueryer struct {
	queryer financialHistoryQueryer
	calls   []recordedFinancialHistoryQuery
}

func (queryer *recordingFinancialHistoryQueryer) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	queryer.calls = append(queryer.calls, recordedFinancialHistoryQuery{query: query, args: append([]any(nil), args...)})
	return queryer.queryer.Query(ctx, query, args...)
}
