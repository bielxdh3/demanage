//go:build integration

package main

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPostgresPiggyBankReadProjectionIntegration(t *testing.T) {
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
		t.Fatalf("begin isolated piggy projection transaction: %v", err)
	}
	rollbackPending := true
	defer func() {
		if rollbackPending {
			if err := tx.Rollback(context.Background()); err != nil {
				t.Errorf("rollback isolated piggy projection transaction: %v", err)
			}
		}
	}()
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO pg_temp`); err != nil {
		t.Fatalf("restrict transaction search path to temporary tables: %v", err)
	}
	fixture, err := os.ReadFile("testdata/piggy-banks-read.integration.sql")
	if err != nil {
		t.Fatalf("read temporary piggy projection fixture: %v", err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("apply transaction-local piggy fixture: %v", err)
		}
	}

	repository := transactionPiggyReadRepository{queryer: tx}
	handler := newPiggyBankReadProjectionHandler(
		testJWTKey,
		&syntheticPiggyReadSessionRepository{},
		repository,
	)
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId":         "synthetic-owner-a",
		"sessionVersion": 0,
		"exp":            4102444800,
	})
	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	cash := request("/piggy-banks/cash-movements")
	if cash.Code != http.StatusOK {
		t.Fatalf("cash movements = %d %s", cash.Code, cash.Body.String())
	}
	cashRows := decodePiggyProjectionArray(t, cash.Body.String())
	if len(cashRows) != 3 {
		t.Fatalf("cash movement count = %d, want 3 linked owner movements: %#v", len(cashRows), cashRows)
	}
	wantCash := []struct {
		typeName string
		amount   string
		date     string
	}{
		{typeName: "deposit", amount: "9999999999.99", date: "2026-05-01"},
		{typeName: "withdraw", amount: "0.1", date: "2026-05-01"},
		{typeName: "deposit", amount: "0.2", date: "2026-05-02"},
	}
	for index, want := range wantCash {
		row := cashRows[index]
		if len(row) != 3 || row["type"] != want.typeName || row["date"] != want.date {
			t.Fatalf("cash movement[%d] = %#v, want type %q date %q", index, row, want.typeName, want.date)
		}
		assertPiggyJSONNumber(t, row["amount"], want.amount)
	}
	assertExpressJSONHeaders(t, cash)

	transactions := request("/piggy-banks/bank-owner-a/transactions")
	if transactions.Code != http.StatusOK {
		t.Fatalf("bank transactions = %d %s", transactions.Code, transactions.Body.String())
	}
	rows := decodePiggyProjectionArray(t, transactions.Body.String())
	wantIDs := []string{"tx-interest", "tx-deposit-late", "tx-unlinked-deposit", "tx-unlinked-withdraw", "tx-wrong-link", "tx-withdraw-linked", "tx-deposit-early"}
	if len(rows) != len(wantIDs) {
		t.Fatalf("bank transaction count = %d, want %d: %#v", len(rows), len(wantIDs), rows)
	}
	for index, wantID := range wantIDs {
		if len(rows[index]) != 14 || rows[index]["id"] != wantID {
			t.Fatalf("transaction[%d] = %#v, want full projection for %q", index, rows[index], wantID)
		}
	}
	interest := rows[0]
	if interest["piggyBankId"] != "bank-owner-a" || interest["type"] != "interest" || interest["source"] != "yield" || interest["date"] != "2026-05-03" || interest["createdAt"] != "2026-05-03T12:01:02.123Z" {
		t.Fatalf("interest identity/date projection = %#v", interest)
	}
	assertPiggyJSONNumber(t, interest["amount"], "0.01")
	assertPiggyJSONNumber(t, interest["cdiRate"], "0.00123456")
	assertPiggyJSONNumber(t, interest["cdiPercent"], "125.5")
	assertPiggyJSONNumber(t, interest["baseBalance"], "10")
	assertPiggyJSONNumber(t, interest["resultingBalance"], "10.01")
	for _, field := range []string{"expenseId", "entryId", "note"} {
		assertPiggyJSONNull(t, interest, field)
	}
	assertPiggyJSONNull(t, rows[1], "entryId")
	assertPiggyJSONNull(t, rows[1], "cdiRate")
	if rows[1]["expenseId"] != "expense-late" || rows[1]["note"] != "" {
		t.Fatalf("linked transaction null/empty values = %#v", rows[1])
	}
	assertExpressJSONHeaders(t, transactions)

	archived := request("/piggy-banks/bank-owner-a-archived/transactions")
	archivedRows := decodePiggyProjectionArray(t, archived.Body.String())
	if archived.Code != http.StatusOK || len(archivedRows) != 1 || archivedRows[0]["id"] != "tx-archived" {
		t.Fatalf("owned archived bank = %d %s", archived.Code, archived.Body.String())
	}
	empty := request("/piggy-banks/bank-owner-a-empty/transactions")
	if empty.Code != http.StatusOK || empty.Body.String() != "[]" {
		t.Fatalf("owned bank without transactions = %d %s, want 200 []", empty.Code, empty.Body.String())
	}
	for _, bankID := range []string{"bank-foreign", "bank-missing"} {
		missing := request("/piggy-banks/" + bankID + "/transactions")
		if missing.Code != http.StatusNotFound || missing.Body.String() != `{"error":"Cofre não encontrado"}` {
			t.Fatalf("%s transaction history = %d %s, want owner-scoped 404", bankID, missing.Code, missing.Body.String())
		}
	}

	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback isolated piggy projection transaction: %v", err)
	}
	rollbackPending = false
}

type transactionPiggyReadRepository struct {
	queryer piggyReadQueryer
}

func (repository transactionPiggyReadRepository) ListPiggyCashMovements(ctx context.Context, userID string) ([]json.RawMessage, error) {
	return listPiggyCashMovements(ctx, repository.queryer, userID)
}

func (repository transactionPiggyReadRepository) ListPiggyBankTransactions(ctx context.Context, userID, bankID string) ([]json.RawMessage, bool, error) {
	return listPiggyBankTransactions(ctx, repository.queryer, userID, bankID)
}

type syntheticPiggyReadSessionRepository struct{}

func (syntheticPiggyReadSessionRepository) FindUserSessionVersion(_ context.Context, userID string) (int64, bool, error) {
	return 0, userID == "synthetic-owner-a", nil
}

func decodePiggyProjectionArray(t *testing.T, body string) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	var rows []map[string]any
	if err := decoder.Decode(&rows); err != nil {
		t.Fatalf("decode piggy projection response: %v", err)
	}
	return rows
}

func assertPiggyJSONNumber(t *testing.T, value any, want string) {
	t.Helper()
	gotNumber, ok := value.(json.Number)
	if !ok {
		t.Fatalf("value = %#v, want exact JSON number %s", value, want)
	}
	got, ok := new(big.Rat).SetString(gotNumber.String())
	if !ok {
		t.Fatalf("JSON number %q is not an exact rational", gotNumber)
	}
	wantValue, ok := new(big.Rat).SetString(want)
	if !ok || got.Cmp(wantValue) != 0 {
		t.Fatalf("JSON number = %s, want exact %s", gotNumber, want)
	}
}

func assertPiggyJSONNull(t *testing.T, object map[string]any, field string) {
	t.Helper()
	value, exists := object[field]
	if !exists || value != nil {
		t.Fatalf("field %q = %#v, want present JSON null", field, value)
	}
}
