//go:build integration

package main

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestPostgresPiggyBanksProjectionIntegration(t *testing.T) {
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

	fixture := []string{
		`CREATE TEMP TABLE "PiggyBank" (
			"id" TEXT NOT NULL,
			"userId" TEXT NOT NULL,
			"name" TEXT NOT NULL,
			"goalAmount" DECIMAL(12,2),
			"targetDate" TIMESTAMP(3),
			"monthlyGoal" DECIMAL(12,2) NOT NULL,
			"autoDebit" BOOLEAN NOT NULL,
			"autoDebitDay" INTEGER NOT NULL,
			"isEmergency" BOOLEAN NOT NULL,
			"yieldEnabled" BOOLEAN NOT NULL,
			"cdiPercent" DECIMAL(8,4) NOT NULL,
			"interestAccruedThrough" TIMESTAMP(3),
			"archivedAt" TIMESTAMP(3),
			"completedAt" TIMESTAMP(3),
			"createdAt" TIMESTAMP(3) NOT NULL,
			"updatedAt" TIMESTAMP(3) NOT NULL
		) ON COMMIT DROP`,
		`CREATE TEMP TABLE "PiggyTransaction" (
			"id" TEXT NOT NULL,
			"piggyBankId" TEXT NOT NULL,
			"userId" TEXT NOT NULL,
			"type" TEXT NOT NULL,
			"source" TEXT NOT NULL,
			"amount" DECIMAL(12,2) NOT NULL,
			"date" TIMESTAMP(3) NOT NULL
		) ON COMMIT DROP`,
		`INSERT INTO "PiggyBank" VALUES
			('bank-active', 'synthetic-owner-a', 'Reserva', 100.00, '2027-02-03 12:00:00.000', 0.10, TRUE, 31, FALSE, TRUE, 100.2500, '2026-11-04 12:00:00.000', NULL, NULL, '2026-02-03 04:05:06.007', '2026-11-04 05:06:07.089'),
			('bank-archived', 'synthetic-owner-a', 'Arquivado', 0.00, NULL, 0.00, FALSE, 1, TRUE, FALSE, 0.0000, NULL, '2026-11-05 09:08:07.006', '2026-08-01 00:00:00.000', '2026-02-02 04:05:06.007', '2026-11-05 09:08:07.006'),
			('bank-empty', 'synthetic-owner-a', 'Sem meta', NULL, NULL, 0.00, FALSE, 1, FALSE, FALSE, 0.0000, NULL, NULL, NULL, '2026-02-01 04:05:06.007', '2026-02-01 04:05:06.007'),
			('bank-foreign', 'synthetic-owner-b', 'Outro titular', 200.00, NULL, 5.00, FALSE, 5, FALSE, FALSE, 0.0000, NULL, NULL, NULL, '2026-01-01 00:00:00.000', '2026-01-01 00:00:00.000')`,
		`INSERT INTO "PiggyTransaction" VALUES
			('tx-deposit', 'bank-active', 'synthetic-owner-a', 'deposit', 'auto_debit', 100.01, '2026-10-02 12:00:00.000'),
			('tx-withdraw', 'bank-active', 'synthetic-owner-a', 'withdraw', 'manual', 0.02, '2026-10-03 12:00:00.000'),
			('tx-interest', 'bank-active', 'synthetic-owner-a', 'interest', 'manual', 0.01, '2026-10-04 12:00:00.000'),
			('tx-wrong-owner', 'bank-active', 'synthetic-owner-b', 'deposit', 'manual', 999.99, '2026-10-05 12:00:00.000'),
			('tx-archived', 'bank-archived', 'synthetic-owner-a', 'deposit', 'manual', 5.00, '2026-09-01 12:00:00.000'),
			('tx-foreign', 'bank-foreign', 'synthetic-owner-b', 'deposit', 'manual', 50.00, '2026-01-02 12:00:00.000')`,
	}
	for _, statement := range fixture {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("apply transaction-local piggy fixture: %v", err)
		}
	}

	// 02:00 UTC is still the prior civil month in America/Sao_Paulo.
	now := time.Date(2026, time.November, 1, 2, 0, 0, 0, time.UTC)
	active, err := queryPiggyBanks(ctx, tx, "synthetic-owner-a", false, now)
	if err != nil {
		t.Fatalf("query active synthetic piggy banks: %v", err)
	}
	if got, want := piggyBankIDs(active), []string{"bank-active", "bank-empty"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("active owner/order = %#v, want %#v", got, want)
	}
	bank := active[0]
	if bank.ID != "bank-active" || bank.GoalAmount == nil || *bank.GoalAmount != money(10000) ||
		bank.TargetDate == nil || *bank.TargetDate != "2027-02-03" || bank.MonthlyGoal != money(10) ||
		bank.CDIPercent != piggyCDIPercent(1002500) || bank.InterestAccruedThrough == nil || *bank.InterestAccruedThrough != "2026-11-04" {
		t.Fatalf("active piggy goal/date/decimal projection = %#v", bank)
	}
	if !bank.AutoDebitPaidThisMonth || bank.Balance != money(10000) || bank.Progress != 1 || bank.Remaining != 0 {
		t.Fatalf("active piggy derived values = %#v", bank)
	}
	if bank.CreatedAt != "2026-02-03T04:05:06.007Z" || bank.UpdatedAt != "2026-11-04T05:06:07.089Z" || bank.ArchivedAt != nil || bank.CompletedAt != nil {
		t.Fatalf("active piggy timestamp/null values = %#v", bank)
	}
	empty := active[1]
	if empty.GoalAmount != nil || empty.TargetDate != nil || empty.InterestAccruedThrough != nil || empty.Balance != 0 || empty.Remaining != 0 || empty.AutoDebitPaidThisMonth {
		t.Fatalf("empty piggy null/default values = %#v", empty)
	}

	withArchived, err := queryPiggyBanks(ctx, tx, "synthetic-owner-a", true, now)
	if err != nil {
		t.Fatalf("query archived synthetic piggy banks: %v", err)
	}
	if got, want := piggyBankIDs(withArchived), []string{"bank-active", "bank-archived", "bank-empty"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("includeArchived owner/order = %#v, want %#v", got, want)
	}
	archived := withArchived[1]
	if archived.GoalAmount != nil || archived.ArchivedAt == nil || *archived.ArchivedAt != "2026-11-05T09:08:07.006Z" || archived.CompletedAt == nil || *archived.CompletedAt != "2026-08-01T00:00:00.000Z" || archived.Balance != money(500) {
		t.Fatalf("archived piggy projection = %#v", archived)
	}

	foreign, err := queryPiggyBanks(ctx, tx, "synthetic-owner-b", true, now)
	if err != nil {
		t.Fatalf("query other synthetic owner: %v", err)
	}
	if got, want := piggyBankIDs(foreign), []string{"bank-foreign"}; !reflect.DeepEqual(got, want) || foreign[0].Balance != money(5000) {
		t.Fatalf("other-owner projection = %#v, want only bank-foreign at 50.00", foreign)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback isolated projection transaction: %v", err)
	}
	rollbackPending = false
}

func piggyBankIDs(banks []piggyBankResponse) []string {
	ids := make([]string, len(banks))
	for index, bank := range banks {
		ids[index] = bank.ID
	}
	return ids
}
