package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

type fakeExpenseListRepository struct {
	userID string
	items  []expense
}

func (repository *fakeExpenseListRepository) ListExpenses(_ context.Context, userID string) ([]expense, error) {
	repository.userID = userID
	return repository.items, nil
}

type fakeEntryListRepository struct {
	userID string
	items  []json.RawMessage
}

func (repository *fakeEntryListRepository) ListEntries(_ context.Context, userID string) ([]json.RawMessage, error) {
	repository.userID = userID
	return repository.items, nil
}

func TestFinancialReadRoutesReturnArraysAndRequireAuthentication(t *testing.T) {
	repository := &fakeCustomTagRepository{userExists: true}
	expenseReader := &fakeExpenseListRepository{items: []expense{}}
	entryReader := &fakeEntryListRepository{items: []json.RawMessage{}}
	handler := newHandler(func(context.Context) error { return nil }, handlerOptions{
		jwtKey:        testJWTKey,
		tagReader:     repository,
		expenseReader: expenseReader,
		entryReader:   entryReader,
	})
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId":         "owner-1",
		"sessionVersion": 0,
		"exp":            4102444800,
	})

	for _, path := range []string{"/expenses", "/expenses/", "/entries", "/entries/"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Body.String() != "[]" {
				t.Fatalf("authenticated response = %d %s, want 200 []", response.Code, response.Body.String())
			}
			assertExpressJSONHeaders(t, response)
			if path == "/expenses" || path == "/expenses/" {
				if expenseReader.userID != "owner-1" {
					t.Fatalf("expense repository received owner %q, want owner-1", expenseReader.userID)
				}
			} else if entryReader.userID != "owner-1" {
				t.Fatalf("entry repository received owner %q, want owner-1", entryReader.userID)
			}

			unauthenticated := httptest.NewRecorder()
			handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, path, nil))
			if unauthenticated.Code != http.StatusUnauthorized {
				t.Fatalf("missing-cookie status = %d, want 401", unauthenticated.Code)
			}
			assertExpressJSONHeaders(t, unauthenticated)
		})
	}
}

func TestCardAndPiggyBankListRoutesAreMountedAndRequireAuthentication(t *testing.T) {
	sessionReader := &fakeCustomTagRepository{userExists: true}
	cardReader := &fakeCardListRepository{items: []cardReadProjection{}}
	piggyBankReader := &fakePiggyBankListRepository{items: []piggyBankResponse{}}
	handler := newHandler(func(context.Context) error { return nil }, handlerOptions{
		jwtKey:          testJWTKey,
		tagReader:       sessionReader,
		cardReader:      cardReader,
		piggyBankListReader: piggyBankReader,
	})
	token := signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId":         "owner-1",
		"sessionVersion": 0,
		"exp":            4102444800,
	})

	for _, path := range []string{"/cards", "/cards/", "/piggy-banks", "/piggy-banks/"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || response.Body.String() != "[]" {
				t.Fatalf("authenticated list response = %d %s, want 200 []", response.Code, response.Body.String())
			}
			assertExpressJSONHeaders(t, response)
			if path == "/cards" || path == "/cards/" {
				if cardReader.userID != "owner-1" {
					t.Fatalf("card repository received owner %q, want owner-1", cardReader.userID)
				}
			} else if piggyBankReader.userID != "owner-1" {
				t.Fatalf("piggy bank repository received owner %q, want owner-1", piggyBankReader.userID)
			}

			unauthenticated := httptest.NewRecorder()
			handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, path, nil))
			if unauthenticated.Code != http.StatusUnauthorized {
				t.Fatalf("missing-cookie status = %d, want 401", unauthenticated.Code)
			}
			assertExpressJSONHeaders(t, unauthenticated)
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/piggy-banks?includeArchived=true", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !piggyBankReader.includeArchived {
		t.Fatalf("mounted includeArchived route = %d %s, includeArchived=%t", response.Code, response.Body.String(), piggyBankReader.includeArchived)
	}
}
