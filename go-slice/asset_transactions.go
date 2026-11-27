package main

import (
	"context"
	"net/http"
	"strings"
)

type assetTransactionListRepository interface {
	ListAssetTransactions(context.Context, string, string) ([]assetTransaction, error)
}

type assetTransaction struct {
	ID             string  `json:"id"`
	Asset          string  `json:"asset"`
	Type           string  `json:"type"`
	Quantity       string  `json:"quantity"`
	CashAmountBRL  string  `json:"cashAmountBrl"`
	FeeAmountBRL   string  `json:"feeAmountBrl"`
	FeePercent     *string `json:"feePercent"`
	CostBasisKnown bool    `json:"costBasisKnown"`
	Date           string  `json:"date"`
	Note           *string `json:"note"`
	ExpenseID      *string `json:"expenseId"`
	EntryID        *string `json:"entryId"`
	CreatedAt      string  `json:"createdAt"`
}

func plainAssetDecimal(value string) string {
	if !strings.Contains(value, ".") {
		return value
	}
	return strings.TrimSuffix(strings.TrimRight(value, "0"), ".")
}

func registerAssetTransactionReads(
	mux *http.ServeMux,
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
	transactionReader assetTransactionListRepository,
) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}

		asset := strings.ToUpper(r.PathValue("asset"))
		if asset != "BTC" && asset != "USD" {
			writeJSON(w, r, http.StatusBadRequest, struct {
				Error string `json:"error"`
			}{"Ativo inválido"})
			return
		}
		if transactionReader == nil {
			writeInternalError(w, r)
			return
		}

		transactions, err := transactionReader.ListAssetTransactions(r.Context(), userID, asset)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if transactions == nil {
			transactions = []assetTransaction{}
		}
		writeJSON(w, r, http.StatusOK, transactions)
	}

	mux.HandleFunc("GET /assets/{asset}/transactions", handler)
	mux.HandleFunc("GET /assets/{asset}/transactions/{$}", handler)
}
