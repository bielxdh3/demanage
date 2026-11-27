//go:build integration

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresPatrimonySettingsProjectionIntegration(t *testing.T) {
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

	fixture, err := os.ReadFile("testdata/patrimony_settings.integration.sql")
	if err != nil {
		t.Fatalf("read patrimony settings fixture: %v", err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("apply transaction-local patrimony settings fixture: %v", err)
		}
	}

	ownerA, err := readPatrimonySettings(ctx, tx, "synthetic-owner-a")
	if err != nil {
		t.Fatalf("query owner A settings: %v", err)
	}
	if ownerA == nil || ownerA.BaseDate != "2026-02-03" || ownerA.OpeningCashBrl != "1234567890123456.78" {
		t.Fatalf("owner A settings = %#v, want Prisma date and exact decimal string", ownerA)
	}
	ownerB, err := readPatrimonySettings(ctx, tx, "synthetic-owner-b")
	if err != nil {
		t.Fatalf("query owner B settings: %v", err)
	}
	if ownerB == nil || ownerB.BaseDate != "2026-12-31" || ownerB.OpeningCashBrl != "0" {
		t.Fatalf("owner B settings = %#v, want its independent exact settings", ownerB)
	}
	missing, err := readPatrimonySettings(ctx, tx, "synthetic-owner-without-settings")
	if err != nil {
		t.Fatalf("query missing settings: %v", err)
	}
	if missing != nil {
		t.Fatalf("missing settings = %#v, want null projection", missing)
	}

	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback isolated projection transaction: %v", err)
	}
	rollbackPending = false
}
