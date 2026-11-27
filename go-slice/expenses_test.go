package main

import (
	"encoding/json"
	"testing"
)

func TestMoneyAndPercentageJSONStayExact(t *testing.T) {
	tests := []struct {
		name string
		value money
		want string
	}{
		{name: "zero amount", value: money(0), want: `0.00`},
		{name: "fractional cents", value: money(1), want: `0.01`},
		{name: "ordinary amount", value: money(123456), want: `1234.56`},
		{name: "negative amount", value: money(-1), want: `-0.01`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatalf("marshal amount: %v", err)
			}
			if string(encoded) != test.want {
				t.Fatalf("amount JSON = %s, want %s", encoded, test.want)
			}
		})
	}

	var decoded money
	if err := json.Unmarshal([]byte(`"1234.56"`), &decoded); err != nil || decoded != money(123456) {
		t.Fatalf("decode decimal amount = %d, %v; want 123456 cents", decoded, err)
	}
	var percent percentage
	if err := json.Unmarshal([]byte(`"33.33"`), &percent); err != nil || percent != percentage(3333) {
		t.Fatalf("decode decimal percentage = %d, %v; want 3333 basis points", percent, err)
	}
	encodedPercent, err := json.Marshal(percentage(3333))
	if err != nil || string(encodedPercent) != `33.33` {
		t.Fatalf("percentage JSON = %s, %v; want 33.33", encodedPercent, err)
	}
}

func TestExpenseJSONKeepsNullFieldsAndEmptyRelations(t *testing.T) {
	value := expense{
		ID:          "expense-zero",
		UserID:      "owner-a",
		Name:        "Sem valor",
		Amount:      money(0),
		Category:    "outro",
		Frequency:   "unica",
		SystemOrigin: "manual",
		CreatedAt:   "2026-02-05T06:07:08.009Z",
		UpdatedAt:   "2026-02-05T06:07:08.009Z",
		Splits:      []expenseSplit{},
		Payments:    []expensePayment{},
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal expense: %v", err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("decode expense JSON: %v", err)
	}
	for field, want := range map[string]string{
		"amount":     `0.00`,
		"cardId":     `null`,
		"customTagId": `null`,
		"dueDay":     `null`,
		"startsAt":   `null`,
		"customTag":  `null`,
		"splits":     `[]`,
		"payments":   `[]`,
	} {
		if string(wire[field]) != want {
			t.Errorf("%s = %s, want %s", field, wire[field], want)
		}
	}
}
