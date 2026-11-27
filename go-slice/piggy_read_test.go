package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakePiggyBankListRepository struct {
	userID          string
	includeArchived bool
	items           []piggyBankResponse
	listCalls       int
}

func (repository *fakePiggyBankListRepository) ListPiggyBanks(
	_ context.Context,
	userID string,
	includeArchived bool,
) ([]piggyBankResponse, error) {
	repository.listCalls++
	repository.userID = userID
	repository.includeArchived = includeArchived
	return repository.items, nil
}

func TestPiggyBankSerializationMatchesExpressMoneyAndDateContract(t *testing.T) {
	now := time.Date(2026, time.November, 1, 2, 0, 0, 0, time.UTC)
	goalAmount := money(10000)
	targetDate := "2027-02-03"
	interestThrough := "2026-11-04"
	record := piggyBankRecord{
		id:                     "bank-a",
		name:                   "Reserva",
		goalAmount:             &goalAmount,
		targetDate:             &targetDate,
		monthlyGoal:            money(10),
		autoDebit:              true,
		autoDebitDay:           31,
		isEmergency:            false,
		yieldEnabled:           true,
		cdiPercent:             piggyCDIPercent(1002500),
		interestAccruedThrough: &interestThrough,
		archivedAt:             nil,
		completedAt:            nil,
		createdAt:              "2026-02-03T04:05:06.007Z",
		updatedAt:              "2026-11-04T05:06:07.089Z",
		transactions: []piggyTransactionRecord{
			{typeName: "deposit", source: "auto_debit", amount: money(10001), date: "2026-10-02"},
			{typeName: "withdraw", source: "manual", amount: money(2), date: "2026-10-03"},
			{typeName: "interest", source: "yield", amount: money(1), date: "2026-10-04"},
		},
	}

	currentMonth, err := piggyMonthInSaoPaulo(now)
	if err != nil || currentMonth != "2026-10" {
		t.Fatalf("São Paulo month at UTC month boundary = %q, %v; want 2026-10", currentMonth, err)
	}
	encoded, err := json.Marshal(serializePiggyBankRecord(record, currentMonth))
	if err != nil {
		t.Fatalf("serialize piggy bank: %v", err)
	}
	want := `{"id":"bank-a","name":"Reserva","goalAmount":100.00,"targetDate":"2027-02-03","monthlyGoal":0.10,"autoDebitPaidThisMonth":true,"autoDebit":true,"autoDebitDay":31,"isEmergency":false,"yieldEnabled":true,"cdiPercent":100.2500,"interestAccruedThrough":"2026-11-04","archivedAt":null,"completedAt":null,"balance":100.00,"progress":1,"remaining":0.00,"createdAt":"2026-02-03T04:05:06.007Z","updatedAt":"2026-11-04T05:06:07.089Z"}`
	if string(encoded) != want {
		t.Fatalf("piggy bank JSON = %s, want %s", encoded, want)
	}
}

func TestPiggyBankSerializationKeepsAbsentGoalAndDatesAsNull(t *testing.T) {
	record := piggyBankRecord{
		id:           "bank-empty",
		name:         "Sem meta",
		monthlyGoal:  money(0),
		autoDebitDay: 1,
		cdiPercent:   piggyCDIPercent(0),
		createdAt:    "2026-02-03T04:05:06.007Z",
		updatedAt:    "2026-02-03T04:05:06.007Z",
		transactions: []piggyTransactionRecord{},
	}
	currentMonth, err := piggyMonthInSaoPaulo(time.Date(2026, time.November, 26, 16, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("resolve São Paulo month: %v", err)
	}
	encoded, err := json.Marshal(serializePiggyBankRecord(record, currentMonth))
	if err != nil {
		t.Fatalf("serialize empty piggy bank: %v", err)
	}
	want := `{"id":"bank-empty","name":"Sem meta","goalAmount":null,"targetDate":null,"monthlyGoal":0.00,"autoDebitPaidThisMonth":false,"autoDebit":false,"autoDebitDay":1,"isEmergency":false,"yieldEnabled":false,"cdiPercent":0.0000,"interestAccruedThrough":null,"archivedAt":null,"completedAt":null,"balance":0.00,"progress":0,"remaining":0.00,"createdAt":"2026-02-03T04:05:06.007Z","updatedAt":"2026-02-03T04:05:06.007Z"}`
	if string(encoded) != want {
		t.Fatalf("empty piggy bank JSON = %s, want %s", encoded, want)
	}
}

func TestPiggyBankListHandlerUsesExactArchiveFlagAndAuthenticatedOwner(t *testing.T) {
	sessionReader := &fakeCustomTagRepository{userExists: true}
	repository := &fakePiggyBankListRepository{items: []piggyBankResponse{}}
	handler := newPiggyBanksHandler(testJWTKey, sessionReader, repository)
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId":         "owner-a",
		"sessionVersion": 0,
		"exp":            4102444800,
	})

	request := httptest.NewRequest(http.MethodGet, "/piggy-banks?includeArchived=true", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("authenticated list response = %d %s, want 200 []", response.Code, response.Body.String())
	}
	if repository.userID != "owner-a" || !repository.includeArchived || repository.listCalls != 1 {
		t.Fatalf("list call owner=%q includeArchived=%t calls=%d", repository.userID, repository.includeArchived, repository.listCalls)
	}
	assertExpressJSONHeaders(t, response)

	request = httptest.NewRequest(http.MethodGet, "/piggy-banks?includeArchived=TRUE", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || repository.includeArchived || repository.listCalls != 2 {
		t.Fatalf("non-exact archive flag response = %d %s; includeArchived=%t calls=%d", response.Code, response.Body.String(), repository.includeArchived, repository.listCalls)
	}
}

func TestPiggyBankListHandlerRejectsDuplicateArchiveFlagsBeforeQuery(t *testing.T) {
	sessionReader := &fakeCustomTagRepository{userExists: true}
	repository := &fakePiggyBankListRepository{items: []piggyBankResponse{}}
	handler := newPiggyBanksHandler(testJWTKey, sessionReader, repository)
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId":         "owner-a",
		"sessionVersion": 0,
		"exp":            4102444800,
	})
	request := httptest.NewRequest(http.MethodGet, "/piggy-banks?includeArchived=true&includeArchived=false", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || response.Body.String() != `{"error":"Parâmetro inválido: includeArchived"}` {
		t.Fatalf("duplicate archive flag = %d %s", response.Code, response.Body.String())
	}
	if repository.listCalls != 0 {
		t.Fatalf("invalid query reached piggy bank repository %d times", repository.listCalls)
	}
}
