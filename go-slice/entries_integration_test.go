//go:build integration

package main

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPostgresEntriesProjectionIntegration(t *testing.T) {
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

	fixture, err := os.ReadFile("testdata/entries.integration.sql")
	if err != nil {
		t.Fatalf("read entry projection fixture: %v", err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("apply transaction-local entry fixture: %v", err)
		}
	}

	projections, err := listEntries(ctx, tx, "synthetic-owner-a")
	if err != nil {
		t.Fatalf("query synthetic entries: %v", err)
	}
	entries := decodeEntryProjectionRows(t, projections)
	wantIDs := []string{"entry-z-salary", "entry-m-wrong-scope", "entry-a-tagged", "entry-empty"}
	gotIDs := make([]string, 0, len(entries))
	byID := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		id := entryProjectionString(t, entry["id"])
		gotIDs = append(gotIDs, id)
		byID[id] = entry
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("visible entry order = %#v, want %#v", gotIDs, wantIDs)
	}

	salary := byID["entry-z-salary"]
	if salary["userId"] != "synthetic-owner-a" || salary["customTagId"] != "tag-foreign" {
		t.Fatalf("salary owner/tag reference = %#v / %#v", salary["userId"], salary["customTagId"])
	}
	for _, field := range []string{"customTag", "notes", "endsAt", "date", "receivedAt", "receivedForMonth", "archivedAt"} {
		assertEntryJSONNull(t, salary, field)
	}
	if salary["startsAt"] != "2026-01-05T00:00:00.000Z" || salary["createdAt"] != "2026-03-01T10:00:00.007Z" {
		t.Fatalf("salary timestamps = startsAt %#v, createdAt %#v", salary["startsAt"], salary["createdAt"])
	}
	if salary["receiptHoldForMonth"] != "2026-12" || salary["receiveDay"] != json.Number("5") {
		t.Fatalf("salary period fields = hold %#v, receiveDay %#v", salary["receiptHoldForMonth"], salary["receiveDay"])
	}
	assertEntryJSONNumber(t, salary["amount"], "9999999999.99")

	holds := entryProjectionArray(t, salary["salaryHolds"])
	if len(holds) != 2 {
		t.Fatalf("salary holds are not month-ordered: %#v", holds)
	}
	firstHold := entryProjectionObject(t, holds[0])
	secondHold := entryProjectionObject(t, holds[1])
	if len(firstHold) != 4 || firstHold["id"] != "hold-nov" || firstHold["entryId"] != "entry-z-salary" || firstHold["month"] != "2026-11" || firstHold["createdAt"] != "2026-11-01T00:00:00.000Z" {
		t.Fatalf("first salary hold projection = %#v", firstHold)
	}
	if len(secondHold) != 4 || secondHold["id"] != "hold-dec" || secondHold["entryId"] != "entry-z-salary" || secondHold["month"] != "2026-12" || secondHold["createdAt"] != "2026-12-01T00:00:00.000Z" {
		t.Fatalf("second salary hold projection = %#v", secondHold)
	}
	versions := entryProjectionArray(t, salary["salaryVersions"])
	if len(versions) != 2 {
		t.Fatalf("salary versions = %#v, want 2", versions)
	}
	firstVersion := entryProjectionObject(t, versions[0])
	secondVersion := entryProjectionObject(t, versions[1])
	if len(firstVersion) != 6 || firstVersion["id"] != "version-nov" || firstVersion["entryId"] != "entry-z-salary" || firstVersion["month"] != "2026-11" || firstVersion["receiveDay"] != json.Number("4") || firstVersion["createdAt"] != "2026-11-01T00:00:00.000Z" {
		t.Fatalf("salary version order/receive day = %#v", versions)
	}
	if len(secondVersion) != 6 || secondVersion["id"] != "version-dec" || secondVersion["entryId"] != "entry-z-salary" || secondVersion["month"] != "2026-12" || secondVersion["receiveDay"] != json.Number("5") || secondVersion["createdAt"] != "2026-12-01T00:00:00.000Z" {
		t.Fatalf("second salary version projection = %#v", secondVersion)
	}
	assertEntryJSONNumber(t, firstVersion["amount"], "1234567890.12")
	assertEntryJSONNumber(t, secondVersion["amount"], "9999999999.99")

	receipts := entryProjectionArray(t, salary["receipts"])
	if len(receipts) != 3 {
		t.Fatalf("salary receipts = %#v, want 3", receipts)
	}
	wantReceiptIDs := []string{"receipt-a", "receipt-b", "receipt-c"}
	wantReceiptMonths := []string{"2026-10", "2026-11", "2026-12"}
	wantReceiptCreatedAt := []string{"2026-11-04T12:00:00.000Z", "2026-11-05T12:00:00.000Z", "2026-11-05T12:00:00.000Z"}
	for index, receipt := range receipts {
		row := entryProjectionObject(t, receipt)
		if len(row) != 6 || row["id"] != wantReceiptIDs[index] || row["entryId"] != "entry-z-salary" || row["month"] != wantReceiptMonths[index] || row["createdAt"] != wantReceiptCreatedAt[index] {
			t.Fatalf("receipt order = %#v, want %#v", receipts, wantReceiptIDs)
		}
		assertEntryJSONNumber(t, row["amount"], []string{"0.1", "0.2", "0.3"}[index])
	}
	if entryProjectionString(t, entryProjectionObject(t, receipts[0])["receivedAt"]) != "2026-11-05T12:00:00.000Z" {
		t.Fatalf("receipt timestamp = %#v", receipts[0])
	}

	wrongScope := byID["entry-m-wrong-scope"]
	if wrongScope["customTagId"] != "tag-expense" {
		t.Fatalf("non-income custom tag was exposed: %#v", wrongScope)
	}
	assertEntryJSONNull(t, wrongScope, "customTag")
	validTag := entryProjectionObject(t, byID["entry-a-tagged"]["customTag"])
	if len(validTag) != 3 || validTag["id"] != "tag-income" || validTag["name"] != "Extra" || validTag["color"] != "#AABBCC" {
		t.Fatalf("income custom tag projection = %#v", validTag)
	}
	if byID["entry-a-tagged"]["notes"] != "" || byID["entry-a-tagged"]["date"] != "2026-11-20T12:00:00.000Z" {
		t.Fatalf("empty text/date semantics = %#v", byID["entry-a-tagged"])
	}
	empty := byID["entry-empty"]
	for _, field := range []string{"customTagId", "customTag", "notes", "receiveDay", "startsAt", "endsAt", "date", "receiptHoldForMonth", "receivedForMonth", "receivedAt", "archivedAt"} {
		assertEntryJSONNull(t, empty, field)
	}
	for _, field := range []string{"salaryHolds", "salaryVersions", "receipts"} {
		if values := entryProjectionArray(t, empty[field]); len(values) != 0 {
			t.Errorf("empty entry %s = %#v, want []", field, values)
		}
	}
	assertEntryJSONNumber(t, empty["amount"], "0.1")

	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback isolated projection transaction: %v", err)
	}
	rollbackPending = false
}

func decodeEntryProjectionRows(t *testing.T, projections []json.RawMessage) []map[string]any {
	t.Helper()
	encoded, err := json.Marshal(projections)
	if err != nil {
		t.Fatalf("marshal entry projections: %v", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.UseNumber()
	var entries []map[string]any
	if err := decoder.Decode(&entries); err != nil {
		t.Fatalf("decode entry projections: %v", err)
	}
	return entries
}

func entryProjectionObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value = %#v, want JSON object", value)
	}
	return object
}

func entryProjectionArray(t *testing.T, value any) []any {
	t.Helper()
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want JSON array", value)
	}
	return array
}

func entryProjectionString(t *testing.T, value any) string {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("value = %#v, want JSON string", value)
	}
	return text
}

func assertEntryJSONNull(t *testing.T, object map[string]any, field string) {
	t.Helper()
	value, exists := object[field]
	if !exists || value != nil {
		t.Fatalf("field %q = %#v, want present JSON null", field, value)
	}
}

func assertEntryJSONNumber(t *testing.T, value any, want string) {
	t.Helper()
	gotNumber, ok := value.(json.Number)
	if !ok {
		t.Fatalf("value = %#v, want JSON number", value)
	}
	got, ok := new(big.Rat).SetString(gotNumber.String())
	if !ok {
		t.Fatalf("JSON number %q is not an exact rational", gotNumber)
	}
	wantValue, ok := new(big.Rat).SetString(want)
	if !ok || got.Cmp(wantValue) != 0 {
		t.Fatalf("JSON number = %s, want exact value %s", gotNumber, want)
	}
}
