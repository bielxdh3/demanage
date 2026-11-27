package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type fakeCardListRepository struct {
	userID    string
	items     []cardReadProjection
	listCalls int
}

func (repository *fakeCardListRepository) ListCards(_ context.Context, userID string) ([]cardReadProjection, error) {
	repository.listCalls++
	repository.userID = userID
	return repository.items, nil
}

func TestCardReadSerializationMatchesExpressFieldsAndUsesInjectedTime(t *testing.T) {
	now := time.Date(2026, time.June, 6, 2, 59, 59, 0, time.UTC)
	card := cardReadRecord{
		ID:             "card-a",
		Name:           "Cartão azul",
		Limit:          cardReadMoney(10000),
		ClosingDay:     5,
		CreatedAt:      cardReadTime("2026-05-01T12:00:00.000Z"),
		UpdatedAt:      cardReadTime("2026-05-02T12:00:00.000Z"),
		LastInvoicedOn: cardReadTimePtr("2026-05-05T12:00:00.000Z"),
		ExpiresAt:      cardReadTimePtr("2026-06-06T02:59:59.000Z"),
	}

	projected, err := serializeCardAt(card, money(3333), now)
	if err != nil {
		t.Fatalf("serialize card at reference time: %v", err)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal card projection: %v", err)
	}
	want := `{"id":"card-a","name":"Cartão azul","limit":100.00,"closingDay":5,"pendingClosingDay":null,"archivedAt":null,"expiresAt":"2026-06-06T02:59:59.000Z","lastInvoicedOn":"2026-05-05T12:00:00.000Z","lastBillingProcessedAt":null,"createdAt":"2026-05-01T12:00:00.000Z","updatedAt":"2026-05-02T12:00:00.000Z","committed":33.33,"available":66.67,"maxChargesPerCycle":{"unica":1,"mensal":1,"semanal":5},"expired":false}`
	if string(encoded) != want {
		t.Fatalf("card JSON = %s, want %s", encoded, want)
	}
}

func TestCardReadCommitmentCountsSplitRecurringAndLateOneOffChargesInSaoPaulo(t *testing.T) {
	card := cardReadRecord{
		ID:                     "card-a",
		ClosingDay:             5,
		LastInvoicedOn:         cardReadTimePtr("2026-05-05T12:00:00.000Z"),
		LastBillingProcessedAt: cardReadTimePtr("2026-05-06T12:00:00.000Z"),
		CreatedAt:              cardReadTime("2026-01-01T12:00:00.000Z"),
	}
	asOf := time.Date(2026, time.June, 6, 2, 59, 59, 0, time.UTC)
	cardID := "card-a"
	expenses := []cardReadExpense{
		{
			Amount:    money(2000),
			Frequency: "mensal",
			DueDay:    cardReadIntPtr(10),
			StartsAt:  cardReadTimePtr("2026-05-10T00:00:00.000Z"),
			CreatedAt: cardReadTime("2026-05-01T12:00:00.000Z"),
			Splits:    []cardReadSplit{{Kind: "card", CardID: &cardID, Amount: money(1001)}},
		},
		{
			Amount:    money(1),
			Frequency: "semanal",
			StartsAt:  cardReadTimePtr("2026-05-06T00:00:00.000Z"),
			CreatedAt: cardReadTime("2026-05-01T12:00:00.000Z"),
			CardID:    &cardID,
		},
		{
			Amount:     money(325),
			Frequency:  "unica",
			OccurredAt: cardReadTimePtr("2026-06-06T02:30:00.000Z"),
			CreatedAt:  cardReadTime("2026-06-06T02:30:00.000Z"),
			CardID:     &cardID,
		},
		{
			Amount:     money(400),
			Frequency:  "unica",
			OccurredAt: cardReadTimePtr("2026-05-05T12:00:00.000Z"),
			CreatedAt:  cardReadTime("2026-05-08T12:00:00.000Z"),
			CardID:     &cardID,
		},
		{
			Amount:     money(99999),
			Frequency:  "unica",
			IsInvoice:  true,
			OccurredAt: cardReadTimePtr("2026-06-01T12:00:00.000Z"),
			CreatedAt:  cardReadTime("2026-06-01T12:00:00.000Z"),
			CardID:     &cardID,
		},
	}

	committed, err := committedByCardAt([]cardReadRecord{card}, expenses, asOf)
	if err != nil {
		t.Fatalf("calculate card commitment: %v", err)
	}
	if got, want := committed[card.ID], money(1731); got != want {
		t.Fatalf("committed = %d cents, want %d cents", got, want)
	}
}

func TestCardReadCommitmentUsesHistoricalTermsAcrossSkippedClosing(t *testing.T) {
	cardID := "card-a"
	card := cardReadRecord{
		ID:                     cardID,
		ClosingDay:             10,
		LastInvoicedOn:         cardReadTimePtr("2026-04-10T12:00:00.000Z"),
		LastBillingProcessedAt: cardReadTimePtr("2026-04-11T12:00:00.000Z"),
		MinimumNextClosingOn:   cardReadTimePtr("2026-06-10T12:00:00.000Z"),
		CreatedAt:              cardReadTime("2026-01-01T12:00:00.000Z"),
	}
	expense := cardReadExpense{
		Amount:    money(2500),
		Frequency: "mensal",
		CardID:    &cardID,
		DueDay:    cardReadIntPtr(15),
		StartsAt:  cardReadTimePtr("2026-02-15T00:00:00.000Z"),
		CreatedAt: cardReadTime("2026-02-01T12:00:00.000Z"),
		ObligationVersions: []cardReadObligationVersion{
			{
				Sequence:       1,
				EffectiveMonth: "2026-01",
				Amount:         money(1000),
				CardID:         &cardID,
				Splits:         []cardReadVersionSplit{{Kind: "card", CardID: &cardID, Amount: money(1000), Percent: "100"}},
			},
			{
				Sequence:       2,
				EffectiveMonth: "2026-05",
				Amount:         money(2500),
				CardID:         &cardID,
				Splits:         []cardReadVersionSplit{{Kind: "card", CardID: &cardID, Amount: money(2500), Percent: "100"}},
			},
		},
	}

	committed, err := committedByCardAt(
		[]cardReadRecord{card},
		[]cardReadExpense{expense},
		time.Date(2026, time.May, 15, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("calculate card commitment across skipped closing: %v", err)
	}
	if got, want := committed[card.ID], money(3500); got != want {
		t.Fatalf("committed = %d cents, want %d cents", got, want)
	}
	maxCharges, err := cardReadMaxChargesForWindow(cardReadCycleWindow{
		periodStartKey: "2026-04-10", closingKey: "2026-06-10", includePeriodStart: false,
	})
	if err != nil {
		t.Fatalf("calculate maxima for skipped closing cycle: %v", err)
	}
	if want := (cardReadMaxCharges{Unica: 1, Mensal: 2, Semanal: 9}); maxCharges != want {
		t.Fatalf("skipped closing cycle maxima = %#v, want %#v", maxCharges, want)
	}
}

func cardReadTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func cardReadTimePtr(value string) *time.Time {
	parsed := cardReadTime(value)
	return &parsed
}

func cardReadMoney(value int64) *money {
	parsed := money(value)
	return &parsed
}

func cardReadIntPtr(value int32) *int32 {
	return &value
}
