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

func TestPostgresExpensesIntegration(t *testing.T) {
	databaseURL := requireGitHubPostgresDSN(t)
	database := newPostgresDatabase(databaseURL)
	defer database.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.pool()
	if err != nil {
		t.Fatalf("create PostgreSQL pool: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin isolated fixture transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	fixture, err := os.ReadFile("testdata/expenses.integration.sql")
	if err != nil {
		t.Fatalf("read synthetic expense fixture: %v", err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("apply isolated synthetic fixture: %v", err)
		}
	}

	expenses, err := queryExpenses(ctx, tx, "synthetic-owner-a")
	if err != nil {
		t.Fatalf("query synthetic expenses: %v", err)
	}
	if len(expenses) != 2 || expenses[0].ID != "manual-a-new" || expenses[1].ID != "manual-a-zero" {
		t.Fatalf("owner filtering and createdAt order = %#v", expenseIDs(expenses))
	}
	if expenses[0].Amount != money(123456) || expenses[0].CustomTag == nil ||
		expenses[0].CustomTag.ID != "tag-a-expense" || expenses[0].CustomTag.Name != "Moradia" ||
		expenses[0].CustomTag.Color != "#112233" {
		t.Fatalf("new expense amount or custom tag = %#v", expenses[0])
	}
	if expenses[0].DueDay == nil || *expenses[0].DueDay != 5 ||
		expenses[0].StartsAt == nil || *expenses[0].StartsAt != "2026-02-01T00:00:00.000Z" ||
		expenses[0].EndsAt == nil || *expenses[0].EndsAt != "2026-12-31T23:59:59.000Z" {
		t.Fatalf("expense schedule = %#v", expenses[0])
	}
	if len(expenses[0].Splits) != 3 || expenses[0].Splits[0].Kind != "card" ||
		expenses[0].Splits[0].Percent != percentage(6000) ||
		expenses[0].Splits[0].Amount != money(74074) ||
		expenses[0].Splits[0].CardName == nil || *expenses[0].Splits[0].CardName != "Azul" ||
		expenses[0].Splits[1].Kind != "card" ||
		expenses[0].Splits[1].Percent != percentage(2000) ||
		expenses[0].Splits[1].Amount != money(24691) ||
		expenses[0].Splits[1].CardName == nil || *expenses[0].Splits[1].CardName != "Verde" ||
		expenses[0].Splits[2].Kind != "pix" || expenses[0].Splits[2].CardID != nil ||
		expenses[0].Splits[2].Percent != percentage(2000) ||
		expenses[0].Splits[2].Amount != money(24691) || expenses[0].Splits[2].CardName != nil {
		t.Fatalf("split order and exact amounts = %#v", expenses[0].Splits)
	}
	if len(expenses[0].Payments) != 3 || expenses[0].Payments[0].Amount != money(500) ||
		expenses[0].Payments[0].CreatedAt != "2026-02-01T10:00:00.000Z" ||
		expenses[0].Payments[1].Amount != money(12345) ||
		expenses[0].Payments[1].CreatedAt != "2026-02-01T10:01:00.000Z" ||
		expenses[0].Payments[2].Amount != money(10) ||
		expenses[0].Payments[2].PaidAt != "2026-02-02T10:00:00.000Z" {
		t.Fatalf("payment order and exact amounts = %#v", expenses[0].Payments)
	}

	zero := expenses[1]
	if zero.Amount != money(0) || zero.DueDay != nil || zero.CardID != nil ||
		zero.CustomTag != nil || zero.StartsAt != nil || len(zero.Splits) != 0 || zero.Splits == nil ||
		len(zero.Payments) != 0 || zero.Payments == nil {
		t.Fatalf("zero, null, and empty relation values = %#v", zero)
	}
	wire, err := json.Marshal(zero)
	if err != nil {
		t.Fatalf("marshal empty expense projection: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(wire, &fields); err != nil {
		t.Fatalf("decode empty expense projection: %v", err)
	}
	if string(fields["amount"]) != "0.00" || string(fields["customTag"]) != "null" ||
		string(fields["splits"]) != "[]" || string(fields["payments"]) != "[]" {
		t.Fatalf("empty expense wire shape = %s", wire)
	}
}

func expenseIDs(expenses []expense) []string {
	ids := make([]string, len(expenses))
	for index, item := range expenses {
		ids[index] = item.ID
	}
	return ids
}
