package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

type fakeAssetTransactionListRepository struct {
	userID string
	asset  string
	items  []assetTransaction
	err    error
	calls  int
}

func (repository *fakeAssetTransactionListRepository) ListAssetTransactions(_ context.Context, userID, asset string) ([]assetTransaction, error) {
	repository.calls++
	repository.userID = userID
	repository.asset = asset
	return repository.items, repository.err
}

func TestPlainAssetDecimalsMatchExpressSerialization(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "1.234500000000", want: "1.2345"},
		{value: "9999999999.99999999", want: "9999999999.99999999"},
		{value: "0.000000000001", want: "0.000000000001"},
		{value: "0.00000000", want: "0"},
		{value: "1000.00000000", want: "1000"},
	}
	for _, test := range tests {
		if got := plainAssetDecimal(test.value); got != test.want {
			t.Errorf("plainAssetDecimal(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestAssetTransactionsProjectionMatchesExpressSerialization(t *testing.T) {
	repository := &fakeAssetTransactionListRepository{items: []assetTransaction{{
		ID: "tx-1", Asset: "BTC", Type: "BUY", Quantity: "0.000000000001",
		CashAmountBRL: "123.45678901", FeeAmountBRL: "0.00000001", FeePercent: nil,
		CostBasisKnown: true, Date: "2026-11-20T09:30:00.123Z", Note: nil,
		ExpenseID: assetStringPointer("expense-1"), EntryID: nil, CreatedAt: "2026-11-20T10:00:00.456Z",
	}}}
	response := serveAssetTransactions(t, repository, "/assets/btc/transactions")
	want := `[{"id":"tx-1","asset":"BTC","type":"BUY","quantity":"0.000000000001","cashAmountBrl":"123.45678901","feeAmountBrl":"0.00000001","feePercent":null,"costBasisKnown":true,"date":"2026-11-20T09:30:00.123Z","note":null,"expenseId":"expense-1","entryId":null,"createdAt":"2026-11-20T10:00:00.456Z"}]`
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Fatalf("response = %d %s, want 200 %s", response.Code, response.Body.String(), want)
	}
	if repository.calls != 1 || repository.userID != "owner-1" || repository.asset != "BTC" {
		t.Fatalf("repository request = calls %d owner %q asset %q, want owner-1 BTC", repository.calls, repository.userID, repository.asset)
	}
	assertExpressJSONHeaders(t, response)
}

func TestAssetTransactionsRejectsInvalidAssetBeforeQuery(t *testing.T) {
	repository := &fakeAssetTransactionListRepository{}
	response := serveAssetTransactions(t, repository, "/assets/ETH/transactions")
	if response.Code != http.StatusBadRequest || response.Body.String() != `{"error":"Ativo inválido"}` {
		t.Fatalf("response = %d %s, want 400 invalid asset", response.Code, response.Body.String())
	}
	if repository.calls != 0 {
		t.Fatalf("invalid asset reached repository %d times", repository.calls)
	}
}

func TestAssetTransactionsReturnsEmptyArray(t *testing.T) {
	repository := &fakeAssetTransactionListRepository{}
	response := serveAssetTransactions(t, repository, "/assets/USD/transactions")
	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("empty response = %d %s, want 200 []", response.Code, response.Body.String())
	}
}

func TestAssetTransactionsSanitizesRepositoryFailures(t *testing.T) {
	repository := &fakeAssetTransactionListRepository{err: errors.New("private database detail")}
	response := serveAssetTransactions(t, repository, "/assets/BTC/transactions")
	if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"Internal server error"}` {
		t.Fatalf("database failure response = %d %s", response.Code, response.Body.String())
	}
}

func serveAssetTransactions(t *testing.T, repository *fakeAssetTransactionListRepository, path string) *httptest.ResponseRecorder {
	t.Helper()
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId":         "owner-1",
		"sessionVersion": 0,
		"exp":            4102444800,
	})
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	registerAssetTransactionReads(mux, testJWTKey, &fakeCustomTagRepository{userExists: true}, repository)
	handler := mux
	handler.ServeHTTP(response, request)
	return response
}

func assetStringPointer(value string) *string {
	return &value
}
