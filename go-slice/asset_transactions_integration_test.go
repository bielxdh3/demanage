//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresAssetTransactionsProjectionIntegration(t *testing.T) {
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

	fixture, err := os.ReadFile("testdata/asset-transactions.integration.sql")
	if err != nil {
		t.Fatalf("read asset transaction fixture: %v", err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("apply transaction-local asset transaction fixture: %v", err)
		}
	}

	ownerABTC, err := listAssetTransactions(ctx, tx, "synthetic-owner-a", "BTC")
	if err != nil {
		t.Fatalf("query synthetic BTC transactions: %v", err)
	}
	encoded, err := json.Marshal(ownerABTC)
	if err != nil {
		t.Fatalf("marshal BTC transaction projection: %v", err)
	}
	want := `[{"id":"tx-btc-date-new","asset":"BTC","type":"BUY","quantity":"1.234567890123","cashAmountBrl":"9999999999.99999999","feeAmountBrl":"0","feePercent":null,"costBasisKnown":true,"date":"2026-11-21T08:00:00.789Z","note":null,"expenseId":null,"entryId":null,"createdAt":"2026-11-20T08:00:00.001Z"},{"id":"tx-btc-same-date-new-created","asset":"BTC","type":"BUY","quantity":"0.000000000001","cashAmountBrl":"123.45678901","feeAmountBrl":"0.00000001","feePercent":"0.00000001","costBasisKnown":true,"date":"2026-11-20T09:30:00.123Z","note":"precision","expenseId":"expense-a","entryId":null,"createdAt":"2026-11-20T10:00:00.456Z"},{"id":"tx-btc-same-date-old-created","asset":"BTC","type":"SELL","quantity":"0.000000000001","cashAmountBrl":"0.1","feeAmountBrl":"0.00000001","feePercent":null,"costBasisKnown":false,"date":"2026-11-20T09:30:00.123Z","note":null,"expenseId":null,"entryId":"entry-a","createdAt":"2026-11-20T09:59:59.999Z"}]`
	if string(encoded) != want {
		t.Fatalf("owner/asset/order/serialization projection = %s, want %s", encoded, want)
	}

	ownerAUSD, err := listAssetTransactions(ctx, tx, "synthetic-owner-a", "USD")
	if err != nil || len(ownerAUSD) != 1 || ownerAUSD[0].ID != "tx-usd-owner-a" {
		t.Fatalf("different asset projection = %#v, err %v", ownerAUSD, err)
	}
	ownerAUSDJSON, err := json.Marshal(ownerAUSD)
	if err != nil || string(ownerAUSDJSON) != `[{"id":"tx-usd-owner-a","asset":"USD","type":"BUY","quantity":"1","cashAmountBrl":"10","feeAmountBrl":"0","feePercent":null,"costBasisKnown":true,"date":"2026-11-20T09:30:00.123Z","note":null,"expenseId":null,"entryId":null,"createdAt":"2026-11-20T10:00:00.000Z"}]` {
		t.Fatalf("decimal normalization = %s, err %v", ownerAUSDJSON, err)
	}
	ownerBBTC, err := listAssetTransactions(ctx, tx, "synthetic-owner-b", "BTC")
	if err != nil || len(ownerBBTC) != 1 || ownerBBTC[0].ID != "tx-btc-owner-b" {
		t.Fatalf("different owner projection = %#v, err %v", ownerBBTC, err)
	}
	empty, err := listAssetTransactions(ctx, tx, "synthetic-owner-c", "BTC")
	if err != nil {
		t.Fatalf("query empty asset transaction list: %v", err)
	}
	emptyJSON, err := json.Marshal(empty)
	if err != nil || string(emptyJSON) != "[]" {
		t.Fatalf("empty projection = %s, err %v; want []", emptyJSON, err)
	}

	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback isolated projection transaction: %v", err)
	}
	rollbackPending = false
}
