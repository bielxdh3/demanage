//go:build integration

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresCardsReadProjectionIntegration(t *testing.T) {
	if os.Getenv("CI") != "true" {
		t.Fatal("card projection integration test requires CI=true")
	}
	database := newPostgresDatabase(requireGitHubPostgresDSN(t))
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.pool()
	if err != nil {
		t.Fatalf("create disposable PostgreSQL pool: %v", err)
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
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO pg_temp"); err != nil {
		t.Fatalf("restrict projection to temporary tables: %v", err)
	}

	fixture, err := os.ReadFile("testdata/cards_read.integration.sql")
	if err != nil {
		t.Fatalf("read synthetic card fixture: %v", err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("apply isolated synthetic card fixture: %v", err)
		}
	}

	projected, err := queryCardsAt(ctx, tx, "synthetic-owner-a", time.Date(2026, time.June, 6, 2, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("project synthetic owner cards: %v", err)
	}
	if len(projected) != 2 || projected[0].ID != "card-a-new" || projected[1].ID != "card-a-old" {
		t.Fatalf("owner filtering and newest-first order = %#v", projected)
	}
	if projected[0].Limit != nil || projected[0].Available != nil || projected[0].Committed != 0 {
		t.Fatalf("unlimited card serialization = %#v", projected[0])
	}
	if projected[1].Committed != money(2357) || projected[1].Available == nil || *projected[1].Available != money(17643) {
		t.Fatalf("split, late adjustment, and owner scoped commitment = %#v", projected[1])
	}
	if projected[1].MaxChargesPerCycle != (cardReadMaxCharges{Unica: 1, Mensal: 1, Semanal: 5}) {
		t.Fatalf("current-cycle maximum = %#v", projected[1].MaxChargesPerCycle)
	}
	if projected[1].PendingClosingDay == nil || *projected[1].PendingClosingDay != 8 || !projected[1].Expired || projected[1].ArchivedAt != nil {
		t.Fatalf("active expired card fields = %#v", projected[1])
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback isolated projection fixture: %v", err)
	}
	rollbackPending = false
}
