package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

type fakePiggyReadSessionRepository struct {
	version int64
	exists  bool
	err     error
	userID  string
}

func (repository *fakePiggyReadSessionRepository) FindUserSessionVersion(_ context.Context, userID string) (int64, bool, error) {
	repository.userID = userID
	return repository.version, repository.exists, repository.err
}

type fakePiggyReadRepository struct {
	cashUserID       string
	cashMovements    []json.RawMessage
	cashErr          error
	cashCalls        int
	transactionUser  string
	transactionBank  string
	bankExists       map[string]bool
	transactions     map[string][]json.RawMessage
	transactionErr   error
	transactionCalls int
}

func TestPiggyBankReadRoutesRequireRepositoryToMount(t *testing.T) {
	sharedHandler := newHandler(func(context.Context) error { return nil }, handlerOptions{
		jwtKey:    testJWTKey,
		tagReader: &fakeCustomTagRepository{userExists: true},
	})
	for _, path := range []string{"/piggy-banks/cash-movements", "/piggy-banks/bank-a/transactions"} {
		response := httptest.NewRecorder()
		sharedHandler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("unconfigured shared handler %s status = %d, want 404", path, response.Code)
		}
	}

	configuredHandler := newHandler(func(context.Context) error { return nil }, handlerOptions{
		jwtKey:          testJWTKey,
		tagReader:       &fakeCustomTagRepository{userExists: true},
		piggyBankReadReader: &fakePiggyReadRepository{},
	})
	response := httptest.NewRecorder()
	configuredHandler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/piggy-banks/cash-movements", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("configured loopback handler status = %d, want 401 for unauthenticated request", response.Code)
	}
}

func (repository *fakePiggyReadRepository) ListPiggyCashMovements(_ context.Context, userID string) ([]json.RawMessage, error) {
	repository.cashCalls++
	repository.cashUserID = userID
	return repository.cashMovements, repository.cashErr
}

func (repository *fakePiggyReadRepository) ListPiggyBankTransactions(_ context.Context, userID, bankID string) ([]json.RawMessage, bool, error) {
	repository.transactionCalls++
	repository.transactionUser = userID
	repository.transactionBank = bankID
	return repository.transactions[bankID], repository.bankExists[bankID], repository.transactionErr
}

func TestPiggyBankReadProjectionRoutesPreserveResultsAndRequireSession(t *testing.T) {
	cash := []json.RawMessage{json.RawMessage(`{"type":"deposit","amount":1234567890.12,"date":"2026-05-01"}`)}
	transactions := []json.RawMessage{json.RawMessage(`{"id":"tx-1","amount":0.1,"date":"2026-05-02"}`)}
	repository := &fakePiggyReadRepository{
		cashMovements: cash,
		bankExists:    map[string]bool{"bank-a": true},
		transactions:  map[string][]json.RawMessage{"bank-a": transactions},
	}
	sessions := &fakePiggyReadSessionRepository{version: 4, exists: true}
	handler := newPiggyBankReadProjectionHandler(testJWTKey, sessions, repository)
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId":         "owner-a",
		"sessionVersion": 4,
		"exp":            4102444800,
	})

	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/piggy-banks/cash-movements", want: `[{"type":"deposit","amount":1234567890.12,"date":"2026-05-01"}]`},
		{path: "/piggy-banks/cash-movements/", want: `[{"type":"deposit","amount":1234567890.12,"date":"2026-05-01"}]`},
		{path: "/piggy-banks/bank-a/transactions", want: `[{"id":"tx-1","amount":0.1,"date":"2026-05-02"}]`},
		{path: "/piggy-banks/bank-a/transactions/", want: `[{"id":"tx-1","amount":0.1,"date":"2026-05-02"}]`},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200: %s", test.path, response.Code, response.Body.String())
		}
		if got := response.Body.String(); got != test.want {
			t.Fatalf("%s response = %s, want %s", test.path, got, test.want)
		}
		assertExpressJSONHeaders(t, response)
	}
	if sessions.userID != "owner-a" || repository.cashUserID != "owner-a" || repository.transactionUser != "owner-a" || repository.transactionBank != "bank-a" {
		t.Fatalf("session/repository scope = session %q, cash owner %q, transaction owner %q bank %q", sessions.userID, repository.cashUserID, repository.transactionUser, repository.transactionBank)
	}

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/piggy-banks/cash-movements", nil))
	if unauthenticated.Code != http.StatusUnauthorized || repository.cashCalls != 2 {
		t.Fatalf("unauthenticated cash read = %d, repository calls %d", unauthenticated.Code, repository.cashCalls)
	}
}

func TestPiggyBankTransactionProjectionDistinguishesEmptyFromMissingOrForeign(t *testing.T) {
	repository := &fakePiggyReadRepository{
		bankExists: map[string]bool{"bank-empty": true, "bank-archived": true},
		transactions: map[string][]json.RawMessage{
			"bank-archived": {json.RawMessage(`{"id":"tx-archived"}`)},
		},
	}
	handler := newPiggyBankReadProjectionHandler(testJWTKey, &fakePiggyReadSessionRepository{exists: true}, repository)
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{"userId": "owner-a", "exp": 4102444800})
	request := func(bankID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/piggy-banks/"+bankID+"/transactions", nil)
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	if response := request("bank-empty"); response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("owned empty bank = %d %s, want 200 []", response.Code, response.Body.String())
	}
	if response := request("bank-archived"); response.Code != http.StatusOK || response.Body.String() != `[{"id":"tx-archived"}]` {
		t.Fatalf("owned archived bank = %d %s", response.Code, response.Body.String())
	}
	for _, bankID := range []string{"bank-foreign", "bank-missing"} {
		response := request(bankID)
		if response.Code != http.StatusNotFound || response.Body.String() != `{"error":"Cofre não encontrado"}` {
			t.Fatalf("%s = %d %s, want 404 missing-bank response", bankID, response.Code, response.Body.String())
		}
	}
}

func TestPiggyBankReadProjectionSQLUsesExpressFiltersAndOrdering(t *testing.T) {
	for _, fragment := range []string{
		`WHERE movement."userId" = $1`,
		`movement.type = 'deposit' AND movement."expenseId" IS NOT NULL`,
		`movement.type = 'withdraw' AND movement."entryId" IS NOT NULL`,
		`ORDER BY movement.date ASC, movement."createdAt" ASC`,
		`'amount', movement.amount`,
		`to_char(movement.date, 'YYYY-MM-DD')`,
	} {
		if !strings.Contains(piggyCashMovementsProjectionSQL, fragment) {
			t.Errorf("cash movements projection is missing %q", fragment)
		}
	}
	for _, fragment := range []string{
		`bank.id = $1 AND bank."userId" = $2`,
		`movement."piggyBankId" = $1 AND movement."userId" = $2`,
		`ORDER BY movement.date DESC, movement."createdAt" DESC`,
		`'amount', movement.amount`,
		`'baseBalance', CASE`,
		`ELSE round(movement."baseBalance", 2)`,
		`'resultingBalance', CASE`,
		`ELSE round(movement."resultingBalance", 2)`,
		`to_char(movement."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`,
	} {
		if !strings.Contains(piggyBankTransactionsExistenceSQL+piggyBankTransactionsProjectionSQL, fragment) {
			t.Errorf("bank transactions projection is missing %q", fragment)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(piggyCashMovementsProjectionSQL), "SELECT") || !strings.HasPrefix(strings.TrimSpace(piggyBankTransactionsProjectionSQL), "SELECT") {
		t.Fatal("piggy read projections must be SELECT-only")
	}
}

func TestPiggyProjectionCollectorPreservesExactJSONAndEmptyArrays(t *testing.T) {
	fixtures := [][]byte{
		[]byte(`{"amount":0.10,"date":"2026-05-01"}`),
		[]byte(`{"amount":1234567890.12,"date":"2026-05-02"}`),
	}
	rows := &syntheticEntryProjectionRows{values: fixtures}
	projections, err := collectPiggyProjections(rows)
	if err != nil {
		t.Fatalf("collect piggy projections: %v", err)
	}
	if !rows.closed {
		t.Fatal("piggy projection rows were not closed")
	}
	encoded, err := json.Marshal(projections)
	if err != nil {
		t.Fatalf("marshal piggy projections: %v", err)
	}
	if !bytes.Equal(encoded, []byte("["+string(fixtures[0])+","+string(fixtures[1])+"]")) {
		t.Fatalf("encoded projections = %s", encoded)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded []map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("decode exact projection JSON: %v", err)
	}
	for index, want := range []string{"0.1", "1234567890.12"} {
		got, ok := decoded[index]["amount"].(json.Number)
		if !ok {
			t.Fatalf("amount[%d] = %#v, want JSON number", index, decoded[index]["amount"])
		}
		gotRat, ok := new(big.Rat).SetString(got.String())
		if !ok {
			t.Fatalf("amount[%d] %q is not an exact rational", index, got)
		}
		wantRat, _ := new(big.Rat).SetString(want)
		if gotRat.Cmp(wantRat) != 0 {
			t.Fatalf("amount[%d] = %s, want %s", index, got, want)
		}
	}

	empty, err := collectPiggyProjections(&syntheticEntryProjectionRows{})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty projections = %#v, %v, want non-nil empty array", empty, err)
	}
	scanFailure := errors.New("synthetic scan failure")
	if _, err := collectPiggyProjections(&syntheticEntryProjectionRows{values: [][]byte{[]byte(`{}`)}, scanErr: scanFailure}); !errors.Is(err, scanFailure) {
		t.Fatalf("scan error = %v, want %v", err, scanFailure)
	}
}
