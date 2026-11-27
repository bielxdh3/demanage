package main

import (
	"context"
	"encoding/json"
	"net/http"
)

type expenseListRepository interface {
	ListExpenses(context.Context, string) ([]expense, error)
}

type entryListRepository interface {
	ListEntries(context.Context, string) ([]json.RawMessage, error)
}

type cardListRepository interface {
	ListCards(context.Context, string) ([]cardReadProjection, error)
}

func registerFinancialReads(
	mux *http.ServeMux,
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
	expenseReader expenseListRepository,
	entryReader entryListRepository,
	cardReader cardListRepository,
	piggyBankReader piggyBankListRepository,
) {
	expensesHandler := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		if expenseReader == nil {
			writeInternalError(w, r)
			return
		}
		expenses, err := expenseReader.ListExpenses(r.Context(), userID)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if expenses == nil {
			expenses = []expense{}
		}
		writeJSON(w, r, http.StatusOK, expenses)
	}
	entriesHandler := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		if entryReader == nil {
			writeInternalError(w, r)
			return
		}
		entries, err := entryReader.ListEntries(r.Context(), userID)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if entries == nil {
			entries = []json.RawMessage{}
		}
		writeJSON(w, r, http.StatusOK, entries)
	}
	cardsHandler := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		if cardReader == nil {
			writeInternalError(w, r)
			return
		}
		cards, err := cardReader.ListCards(r.Context(), userID)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if cards == nil {
			cards = []cardReadProjection{}
		}
		writeJSON(w, r, http.StatusOK, cards)
	}
	piggyBanksHandler := newPiggyBanksHandler(jwtKey, sessionReader, piggyBankReader)

	mux.HandleFunc("GET /expenses", expensesHandler)
	mux.HandleFunc("GET /expenses/{$}", expensesHandler)
	mux.HandleFunc("GET /entries", entriesHandler)
	mux.HandleFunc("GET /entries/{$}", entriesHandler)
	mux.HandleFunc("GET /cards", cardsHandler)
	mux.HandleFunc("GET /cards/{$}", cardsHandler)
	mux.Handle("GET /piggy-banks", piggyBanksHandler)
	mux.Handle("GET /piggy-banks/{$}", piggyBanksHandler)
}

func financialReadUserOrRespond(
	w http.ResponseWriter,
	r *http.Request,
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
) (string, bool) {
	userID, authenticated, err := authenticatedFinancialReadUser(r, jwtKey, sessionReader)
	if err != nil {
		writeInternalError(w, r)
		return "", false
	}
	if !authenticated {
		writeJSON(w, r, http.StatusUnauthorized, struct {
			Error string `json:"error"`
		}{"N\u00e3o autenticado"})
		return "", false
	}
	return userID, true
}
