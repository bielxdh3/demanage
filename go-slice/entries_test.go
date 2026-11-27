package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type syntheticEntryProjectionRows struct {
	values   [][]byte
	position int
	scanErr  error
	rowsErr  error
	closed   bool
}

func (rows *syntheticEntryProjectionRows) Next() bool {
	return rows.position < len(rows.values)
}

func (rows *syntheticEntryProjectionRows) Scan(destinations ...any) error {
	if rows.scanErr != nil {
		return rows.scanErr
	}
	value, ok := destinations[0].(*[]byte)
	if !ok {
		return errors.New("entry projection destination must be *[]byte")
	}
	*value = append((*value)[:0], rows.values[rows.position]...)
	rows.position++
	return nil
}

func (rows *syntheticEntryProjectionRows) Err() error {
	return rows.rowsErr
}

func (rows *syntheticEntryProjectionRows) Close() {
	rows.closed = true
}

func TestEntryProjectionSQLScopesAndOrdersRowsDeterministically(t *testing.T) {
	for _, fragment := range []string{
		`WHERE entry."userId" = $1`,
		`AND entry."archivedAt" IS NULL`,
		`AND entry."systemOrigin" = 'manual'`,
		`ORDER BY entry."createdAt" DESC, entry.id DESC`,
		`LEFT JOIN "CustomTag" AS custom_tag`,
		`ON custom_tag.id = entry."customTagId"`,
		`AND custom_tag."userId" = entry."userId"`,
		`AND custom_tag.scope = 'income'`,
		`ORDER BY hold.month ASC, hold.id ASC`,
		`ORDER BY version.month ASC, version.id ASC`,
		`ORDER BY receipt."receivedAt" ASC, receipt."createdAt" ASC, receipt.id ASC`,
		`'amount', entry.amount`,
		`'amount', version.amount`,
		`'amount', receipt.amount`,
	} {
		if !strings.Contains(entryProjectionSQL, fragment) {
			t.Errorf("entry projection query is missing %q", fragment)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(entryProjectionSQL), "SELECT") {
		t.Fatal("entry projection must use a read-only SELECT")
	}
}

func TestEntryProjectionRowsKeepNestedFieldsAndExactJSONNumbers(t *testing.T) {
	fixtures := [][]byte{
		[]byte(`{"id":"entry-salary","userId":"owner-a","customTagId":"tag-income","name":"Salário","notes":null,"amount":1234567890.12,"type":"salario","frequency":"mensal","receiveDay":5,"startsAt":null,"endsAt":null,"date":null,"receiptHoldForMonth":null,"receivedForMonth":"2026-11","receivedAt":"2026-11-05T12:00:00.000Z","archivedAt":null,"systemOrigin":"manual","createdAt":"2026-10-01T00:00:00.000Z","updatedAt":"2026-11-05T12:00:00.000Z","customTag":{"id":"tag-income","name":"Renda extra","color":"#AABBCC"},"salaryHolds":[{"id":"hold-a","entryId":"entry-salary","month":"2026-12","createdAt":"2026-11-05T12:00:00.000Z"}],"salaryVersions":[{"id":"version-a","entryId":"entry-salary","month":"2026-11","amount":1234567890.12,"receiveDay":5,"createdAt":"2026-11-01T00:00:00.000Z"}],"receipts":[{"id":"receipt-a","entryId":"entry-salary","month":"2026-11","amount":1234567890.12,"receivedAt":"2026-11-05T12:00:00.000Z","createdAt":"2026-11-05T12:00:00.000Z"}]}`),
		[]byte(`{"id":"entry-empty","userId":"owner-a","customTagId":null,"name":"Bônus","notes":"","amount":0.1,"type":"outro","frequency":"unica","receiveDay":null,"startsAt":null,"endsAt":null,"date":"2026-11-20T12:00:00.000Z","receiptHoldForMonth":null,"receivedForMonth":null,"receivedAt":null,"archivedAt":null,"systemOrigin":"manual","createdAt":"2026-10-02T00:00:00.000Z","updatedAt":"2026-10-02T00:00:00.000Z","customTag":null,"salaryHolds":[],"salaryVersions":[],"receipts":[]}`),
	}
	rows := &syntheticEntryProjectionRows{values: fixtures}
	entries, err := collectEntryProjections(rows)
	if err != nil {
		t.Fatalf("collect entry projections: %v", err)
	}
	if !rows.closed {
		t.Fatal("entry projection rows were not closed")
	}

	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal entry projections: %v", err)
	}
	want := `[` + string(fixtures[0]) + `,` + string(fixtures[1]) + `]`
	if !bytes.Equal(encoded, []byte(want)) {
		t.Fatalf("encoded entries = %s, want %s", encoded, want)
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded []map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("decode entry projections: %v", err)
	}
	if got, ok := decoded[0]["amount"].(json.Number); !ok || got.String() != "1234567890.12" {
		t.Fatalf("salary amount = %#v, want exact JSON number 1234567890.12", decoded[0]["amount"])
	}
	if got, ok := decoded[0]["salaryVersions"].([]any)[0].(map[string]any)["amount"].(json.Number); !ok || got.String() != "1234567890.12" {
		t.Fatalf("salary version amount = %#v, want exact JSON number 1234567890.12", decoded[0]["salaryVersions"])
	}
	if got, ok := decoded[0]["receipts"].([]any)[0].(map[string]any)["amount"].(json.Number); !ok || got.String() != "1234567890.12" {
		t.Fatalf("receipt amount = %#v, want exact JSON number 1234567890.12", decoded[0]["receipts"])
	}
	if got, ok := decoded[1]["amount"].(json.Number); !ok || got.String() != "0.1" {
		t.Fatalf("one-off amount = %#v, want exact JSON number 0.1", decoded[1]["amount"])
	}
}

func TestEntryProjectionRowsReturnEmptyArrayAndPropagateErrors(t *testing.T) {
	emptyRows := &syntheticEntryProjectionRows{}
	entries, err := collectEntryProjections(emptyRows)
	if err != nil {
		t.Fatalf("collect empty entry projections: %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Fatalf("empty entries = %#v, want a non-nil empty slice", entries)
	}
	if !emptyRows.closed {
		t.Fatal("empty entry projection rows were not closed")
	}

	scanFailure := errors.New("synthetic scan failure")
	if _, err := collectEntryProjections(&syntheticEntryProjectionRows{values: [][]byte{[]byte(`{}`)}, scanErr: scanFailure}); !errors.Is(err, scanFailure) {
		t.Fatalf("scan error = %v, want %v", err, scanFailure)
	}

	rowsFailure := errors.New("synthetic rows failure")
	if _, err := collectEntryProjections(&syntheticEntryProjectionRows{rowsErr: rowsFailure}); !errors.Is(err, rowsFailure) {
		t.Fatalf("rows error = %v, want %v", err, rowsFailure)
	}
}
