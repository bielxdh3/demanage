package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type cardReadQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type postgresCardRepository struct {
	database *postgresDatabase
}

type cardReadRecord struct {
	ID                     string
	Name                   string
	Limit                  *money
	ClosingDay             int32
	PendingClosingDay      *int32
	ArchivedAt             *time.Time
	ExpiresAt              *time.Time
	LastInvoicedOn         *time.Time
	LastBillingProcessedAt *time.Time
	MinimumNextClosingOn   *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type cardReadSplit struct {
	Kind   string
	CardID *string
	Amount money
}

type cardReadVersionSplit struct {
	Kind    string
	CardID  *string
	Amount  money
	Percent string
}

type cardReadObligationVersion struct {
	Sequence       int
	EffectiveMonth string
	Amount         money
	CardID         *string
	Splits         []cardReadVersionSplit
}

type cardReadExpense struct {
	Amount             money
	CardID             *string
	IsInvoice          bool
	Frequency          string
	DueDay             *int32
	StartsAt           *time.Time
	EndsAt             *time.Time
	OccurredAt         *time.Time
	ArchivedAt         *time.Time
	CreatedAt          time.Time
	Splits             []cardReadSplit
	ObligationVersions []cardReadObligationVersion
}

type cardReadMaxCharges struct {
	Unica   int `json:"unica"`
	Mensal  int `json:"mensal"`
	Semanal int `json:"semanal"`
}

type cardReadProjection struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	Limit                *money             `json:"limit"`
	ClosingDay           int32              `json:"closingDay"`
	PendingClosingDay    *int32             `json:"pendingClosingDay"`
	ArchivedAt           *string            `json:"archivedAt"`
	ExpiresAt            *string            `json:"expiresAt"`
	LastInvoicedOn       *string            `json:"lastInvoicedOn"`
	LastBillingProcessed *string            `json:"lastBillingProcessedAt"`
	CreatedAt            string             `json:"createdAt"`
	UpdatedAt            string             `json:"updatedAt"`
	Committed            money              `json:"committed"`
	Available            *money             `json:"available"`
	MaxChargesPerCycle   cardReadMaxCharges `json:"maxChargesPerCycle"`
	Expired              bool               `json:"expired"`
}

type cardReadCycleWindow struct {
	periodStartKey     string
	closingKey         string
	includePeriodStart bool
}

type cardReadRawVersion struct {
	Sequence       int             `json:"sequence"`
	EffectiveMonth string          `json:"effectiveMonth"`
	Amount         string          `json:"amount"`
	CardID         *string         `json:"cardId"`
	Splits         json.RawMessage `json:"splits"`
}

type cardReadRawVersionSplit struct {
	Kind    string          `json:"kind"`
	CardID  json.RawMessage `json:"cardId"`
	Amount  string          `json:"amount"`
	Percent string          `json:"percent"`
}

type cardReadRawExpenseSplit struct {
	Kind   string  `json:"kind"`
	CardID *string `json:"cardId"`
	Amount string  `json:"amount"`
}

func (repository postgresCardRepository) ListCards(ctx context.Context, userID string) ([]cardReadProjection, error) {
	if repository.database == nil {
		return nil, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, err
	}
	return queryCardsAt(queryContext, pool, userID, time.Now())
}

func queryCardsAt(ctx context.Context, queryer cardReadQueryer, userID string, asOf time.Time) ([]cardReadProjection, error) {
	cards, err := queryCardReadRecords(ctx, queryer, userID)
	if err != nil {
		return nil, err
	}
	expenses, err := queryCardReadExpenses(ctx, queryer, userID)
	if err != nil {
		return nil, err
	}
	committed, err := committedByCardAt(cards, expenses, asOf)
	if err != nil {
		return nil, err
	}
	result := make([]cardReadProjection, 0, len(cards))
	for _, card := range cards {
		projection, err := serializeCardAt(card, committed[card.ID], asOf)
		if err != nil {
			return nil, err
		}
		result = append(result, projection)
	}
	return result, nil
}

func queryCardReadRecords(ctx context.Context, queryer cardReadQueryer, userID string) ([]cardReadRecord, error) {
	rows, err := queryer.Query(ctx, cardReadListQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]cardReadRecord, 0)
	for rows.Next() {
		var card cardReadRecord
		var limit, archivedAt, expiresAt, lastInvoicedOn, lastProcessedAt, minimumNextClosingOn pgtype.Text
		var closingDay int32
		var pendingClosingDay pgtype.Int4
		var createdAt, updatedAt pgtype.Text
		if err := rows.Scan(
			&card.ID,
			&card.Name,
			&limit,
			&closingDay,
			&pendingClosingDay,
			&archivedAt,
			&expiresAt,
			&lastInvoicedOn,
			&lastProcessedAt,
			&minimumNextClosingOn,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		card.ClosingDay = closingDay
		if pendingClosingDay.Valid {
			value := pendingClosingDay.Int32
			card.PendingClosingDay = &value
		}
		if limit.Valid {
			value, err := parseMoney(limit.String)
			if err != nil {
				return nil, fmt.Errorf("parse card limit: %w", err)
			}
			card.Limit = &value
		}
		if card.ArchivedAt, err = parseCardReadNullableTimestamp(archivedAt); err != nil {
			return nil, err
		}
		if card.ExpiresAt, err = parseCardReadNullableTimestamp(expiresAt); err != nil {
			return nil, err
		}
		if card.LastInvoicedOn, err = parseCardReadNullableTimestamp(lastInvoicedOn); err != nil {
			return nil, err
		}
		if card.LastBillingProcessedAt, err = parseCardReadNullableTimestamp(lastProcessedAt); err != nil {
			return nil, err
		}
		if card.MinimumNextClosingOn, err = parseCardReadNullableTimestamp(minimumNextClosingOn); err != nil {
			return nil, err
		}
		card.CreatedAt, err = parseCardReadRequiredTimestamp(createdAt)
		if err != nil {
			return nil, err
		}
		card.UpdatedAt, err = parseCardReadRequiredTimestamp(updatedAt)
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cards, nil
}

func queryCardReadExpenses(ctx context.Context, queryer cardReadQueryer, userID string) ([]cardReadExpense, error) {
	rows, err := queryer.Query(ctx, cardReadCommitmentsQuery, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	expenses := make([]cardReadExpense, 0)
	for rows.Next() {
		var expense cardReadExpense
		var cardID, startsAt, endsAt, occurredAt, archivedAt pgtype.Text
		var amount string
		var dueDay pgtype.Int4
		var createdAt pgtype.Text
		var splitsJSON, versionsJSON []byte
		if err := rows.Scan(
			&amount,
			&cardID,
			&expense.IsInvoice,
			&expense.Frequency,
			&dueDay,
			&startsAt,
			&endsAt,
			&occurredAt,
			&archivedAt,
			&createdAt,
			&splitsJSON,
			&versionsJSON,
		); err != nil {
			return nil, err
		}
		expense.Amount, err = parseMoney(amount)
		if err != nil {
			return nil, fmt.Errorf("parse expense amount for card commitment: %w", err)
		}
		expense.CardID = textPointer(cardID)
		if dueDay.Valid {
			value := dueDay.Int32
			expense.DueDay = &value
		}
		if expense.StartsAt, err = parseCardReadNullableTimestamp(startsAt); err != nil {
			return nil, err
		}
		if expense.EndsAt, err = parseCardReadNullableTimestamp(endsAt); err != nil {
			return nil, err
		}
		if expense.OccurredAt, err = parseCardReadNullableTimestamp(occurredAt); err != nil {
			return nil, err
		}
		if expense.ArchivedAt, err = parseCardReadNullableTimestamp(archivedAt); err != nil {
			return nil, err
		}
		expense.CreatedAt, err = parseCardReadRequiredTimestamp(createdAt)
		if err != nil {
			return nil, err
		}
		expense.Splits, err = parseCardReadExpenseSplits(splitsJSON)
		if err != nil {
			return nil, err
		}
		expense.ObligationVersions, err = parseCardReadObligationVersions(versionsJSON)
		if err != nil {
			return nil, err
		}
		expenses = append(expenses, expense)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return expenses, nil
}

func parseCardReadExpenseSplits(data []byte) ([]cardReadSplit, error) {
	if len(data) == 0 || data[0] != '[' {
		return nil, errors.New("invalid persisted expense splits")
	}
	var raw []cardReadRawExpenseSplit
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse persisted expense splits: %w", err)
	}
	splits := make([]cardReadSplit, 0, len(raw))
	for _, split := range raw {
		amount, err := parseMoney(split.Amount)
		if err != nil {
			return nil, fmt.Errorf("parse persisted expense split amount: %w", err)
		}
		splits = append(splits, cardReadSplit{Kind: split.Kind, CardID: split.CardID, Amount: amount})
	}
	return splits, nil
}

func parseCardReadObligationVersions(data []byte) ([]cardReadObligationVersion, error) {
	if len(data) == 0 || data[0] != '[' {
		return nil, errors.New("invalid persisted expense obligation versions")
	}
	var raw []cardReadRawVersion
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse persisted expense obligation versions: %w", err)
	}
	versions := make([]cardReadObligationVersion, 0, len(raw))
	for _, item := range raw {
		if item.Sequence < 1 || item.EffectiveMonth == "" || !cardReadValidMonth(item.EffectiveMonth) {
			return nil, errors.New("invalid persisted expense obligation version")
		}
		amount, err := parseMoney(item.Amount)
		if err != nil {
			return nil, fmt.Errorf("parse persisted obligation amount: %w", err)
		}
		if len(item.Splits) == 0 || item.Splits[0] != '[' {
			return nil, errors.New("invalid persisted expense obligation version splits")
		}
		var rawSplits []cardReadRawVersionSplit
		if err := json.Unmarshal(item.Splits, &rawSplits); err != nil {
			return nil, fmt.Errorf("parse persisted obligation splits: %w", err)
		}
		splits := make([]cardReadVersionSplit, 0, len(rawSplits))
		var splitTotal money
		for _, rawSplit := range rawSplits {
			cardID, err := parseCardReadSnapshotCardID(rawSplit.CardID)
			if err != nil || !cardReadValidSnapshotSplit(rawSplit, cardID) {
				return nil, errors.New("invalid persisted expense obligation version split")
			}
			splitAmount, err := parseMoney(rawSplit.Amount)
			if err != nil {
				return nil, fmt.Errorf("parse persisted obligation split amount: %w", err)
			}
			splitTotal, err = addCardReadMoney(splitTotal, splitAmount)
			if err != nil {
				return nil, err
			}
			splits = append(splits, cardReadVersionSplit{
				Kind: rawSplit.Kind, CardID: cardID, Amount: splitAmount, Percent: rawSplit.Percent,
			})
		}
		if len(splits) > 0 && splitTotal != amount {
			return nil, errors.New("persisted expense obligation splits do not match amount")
		}
		versions = append(versions, cardReadObligationVersion{
			Sequence: item.Sequence, EffectiveMonth: item.EffectiveMonth, Amount: amount,
			CardID: item.CardID, Splits: splits,
		})
	}
	return versions, nil
}

func parseCardReadSnapshotCardID(data json.RawMessage) (*string, error) {
	if len(data) == 0 {
		return nil, errors.New("missing card id in persisted split")
	}
	if bytes.Equal(data, []byte("null")) {
		return nil, nil
	}
	var cardID string
	if err := json.Unmarshal(data, &cardID); err != nil || cardID == "" {
		return nil, errors.New("invalid card id in persisted split")
	}
	return &cardID, nil
}

func cardReadValidSnapshotSplit(split cardReadRawVersionSplit, cardID *string) bool {
	if !cardReadValidSnapshotDecimal(split.Amount) || !cardReadValidSnapshotDecimal(split.Percent) {
		return false
	}
	if split.Kind == "card" {
		return cardID != nil && *cardID != ""
	}
	return split.Kind == "pix" && cardID == nil
}

func cardReadValidSnapshotDecimal(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) > 2 || !decimalDigits(parts[0]) {
		return false
	}
	return len(parts) == 1 || (len(parts[1]) >= 1 && len(parts[1]) <= 2 && decimalDigits(parts[1]))
}

func cardReadValidMonth(value string) bool {
	if len(value) != 7 || value[4] != '-' || !decimalDigits(value[:4]) || !decimalDigits(value[5:]) {
		return false
	}
	month := value[5:]
	return month >= "01" && month <= "12"
}

func parseCardReadNullableTimestamp(value pgtype.Text) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := parseCardReadTimestamp(value.String)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseCardReadRequiredTimestamp(value pgtype.Text) (time.Time, error) {
	if !value.Valid {
		return time.Time{}, errors.New("missing required card projection timestamp")
	}
	return parseCardReadTimestamp(value.String)
}

func parseCardReadTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse card projection timestamp: %w", err)
	}
	return parsed.UTC(), nil
}

func committedByCardAt(cards []cardReadRecord, expenses []cardReadExpense, asOf time.Time) (map[string]money, error) {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return nil, fmt.Errorf("load billing timezone: %w", err)
	}
	todayKey := asOf.In(location).Format("2006-01-02")
	result := make(map[string]money, len(cards))
	for _, card := range cards {
		window, err := currentCardReadCycleWindow(card, todayKey, location)
		if err != nil {
			return nil, err
		}
		var total money
		for _, expense := range expenses {
			charge, err := cardReadChargeAmount(expense, card.ID, window, location)
			if err != nil {
				return nil, err
			}
			total, err = addCardReadMoney(total, charge)
			if err != nil {
				return nil, err
			}
		}
		if card.LastInvoicedOn != nil && card.LastBillingProcessedAt != nil {
			closedThrough := card.LastInvoicedOn.UTC().Format("2006-01-02")
			late, err := cardReadLateOneOffs(expenses, card.ID, closedThrough, *card.LastBillingProcessedAt, location)
			if err != nil {
				return nil, err
			}
			total, err = addCardReadMoney(total, late)
			if err != nil {
				return nil, err
			}
		}
		result[card.ID] = total
	}
	return result, nil
}

func currentCardReadCycleWindow(card cardReadRecord, todayKey string, location *time.Location) (cardReadCycleWindow, error) {
	periodStartKey := ""
	includePeriodStart := card.LastInvoicedOn == nil
	if card.LastInvoicedOn != nil {
		periodStartKey = card.LastInvoicedOn.UTC().Format("2006-01-02")
	} else {
		periodStartKey = card.CreatedAt.In(location).Format("2006-01-02")
	}
	minimumKey := todayKey
	if card.MinimumNextClosingOn != nil {
		storedMinimum := card.MinimumNextClosingOn.UTC().Format("2006-01-02")
		if storedMinimum > minimumKey {
			minimumKey = storedMinimum
		}
	}
	closingKey, err := cardReadNextClosingOnOrAfter(card.ClosingDay, periodStartKey, minimumKey)
	if err != nil {
		return cardReadCycleWindow{}, err
	}
	return cardReadCycleWindow{
		periodStartKey: periodStartKey, closingKey: closingKey, includePeriodStart: includePeriodStart,
	}, nil
}

func serializeCardAt(card cardReadRecord, committed money, asOf time.Time) (cardReadProjection, error) {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return cardReadProjection{}, fmt.Errorf("load billing timezone: %w", err)
	}
	window, err := currentCardReadCycleWindow(card, asOf.In(location).Format("2006-01-02"), location)
	if err != nil {
		return cardReadProjection{}, err
	}
	maxCharges, err := cardReadMaxChargesForWindow(window)
	if err != nil {
		return cardReadProjection{}, err
	}
	if card.ArchivedAt != nil {
		committed = 0
	}
	var available *money
	if card.Limit != nil {
		value, err := subtractCardReadMoney(*card.Limit, committed)
		if err != nil {
			return cardReadProjection{}, err
		}
		available = &value
	}
	return cardReadProjection{
		ID: card.ID, Name: card.Name, Limit: card.Limit, ClosingDay: card.ClosingDay,
		PendingClosingDay:    card.PendingClosingDay,
		ArchivedAt:           cardReadFormatNullableTimestamp(card.ArchivedAt),
		ExpiresAt:            cardReadFormatNullableTimestamp(card.ExpiresAt),
		LastInvoicedOn:       cardReadFormatNullableTimestamp(card.LastInvoicedOn),
		LastBillingProcessed: cardReadFormatNullableTimestamp(card.LastBillingProcessedAt),
		CreatedAt:            card.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		UpdatedAt:            card.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		Committed:            committed, Available: available, MaxChargesPerCycle: maxCharges,
		Expired: card.ExpiresAt != nil && card.ExpiresAt.Before(asOf),
	}, nil
}

func cardReadFormatNullableTimestamp(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format("2006-01-02T15:04:05.000Z")
	return &formatted
}

func cardReadChargeAmount(expense cardReadExpense, cardID string, window cardReadCycleWindow, location *time.Location) (money, error) {
	if expense.Frequency != "unica" && len(expense.ObligationVersions) > 0 {
		lower, err := cardReadLowerCycleKey(window)
		if err != nil {
			return 0, err
		}
		cursor := lower
		var total money
		for cursor <= window.closingKey {
			month := cursor[:7]
			monthEnd, err := cardReadMonthEnd(month)
			if err != nil {
				return 0, err
			}
			hi := monthEnd
			if window.closingKey < hi {
				hi = window.closingKey
			}
			active := cardReadActiveVersion(expense.ObligationVersions, month)
			if active != nil {
				previousDay, err := cardReadAddDays(cursor, -1)
				if err != nil {
					return 0, err
				}
				occurrences, err := cardReadChargeOccurrences(expense, cardReadCycleWindow{
					periodStartKey: previousDay, closingKey: hi, includePeriodStart: false,
				}, location)
				if err != nil {
					return 0, err
				}
				share, err := cardReadVersionShare(*active, cardID)
				if err != nil {
					return 0, err
				}
				charge, err := multiplyCardReadMoney(share, occurrences)
				if err != nil {
					return 0, err
				}
				total, err = addCardReadMoney(total, charge)
				if err != nil {
					return 0, err
				}
			}
			cursor, err = cardReadAddDays(hi, 1)
			if err != nil {
				return 0, err
			}
		}
		return total, nil
	}
	occurrences, err := cardReadChargeOccurrences(expense, window, location)
	if err != nil || occurrences == 0 {
		return 0, err
	}
	share, err := cardReadExpenseShare(expense, cardID)
	if err != nil {
		return 0, err
	}
	return multiplyCardReadMoney(share, occurrences)
}

func cardReadActiveVersion(versions []cardReadObligationVersion, month string) *cardReadObligationVersion {
	var active *cardReadObligationVersion
	for index := range versions {
		version := &versions[index]
		if version.EffectiveMonth <= month && (active == nil || version.Sequence > active.Sequence) {
			active = version
		}
	}
	return active
}

func cardReadVersionShare(version cardReadObligationVersion, cardID string) (money, error) {
	var total money
	found := false
	for _, split := range version.Splits {
		if split.Kind == "card" && split.CardID != nil && *split.CardID == cardID {
			var err error
			total, err = addCardReadMoney(total, split.Amount)
			if err != nil {
				return 0, err
			}
			found = true
		}
	}
	if found {
		return total, nil
	}
	if version.CardID != nil && *version.CardID == cardID {
		return version.Amount, nil
	}
	return 0, nil
}

func cardReadExpenseShare(expense cardReadExpense, cardID string) (money, error) {
	var total money
	found := false
	for _, split := range expense.Splits {
		if split.Kind == "card" && split.CardID != nil && *split.CardID == cardID {
			var err error
			total, err = addCardReadMoney(total, split.Amount)
			if err != nil {
				return 0, err
			}
			found = true
		}
	}
	if found {
		return total, nil
	}
	if expense.CardID != nil && *expense.CardID == cardID {
		return expense.Amount, nil
	}
	return 0, nil
}

func cardReadChargeOccurrences(expense cardReadExpense, window cardReadCycleWindow, location *time.Location) (int64, error) {
	if expense.IsInvoice {
		return 0, nil
	}
	lower, err := cardReadLowerCycleKey(window)
	if err != nil {
		return 0, err
	}
	if expense.Frequency == "unica" {
		key := expense.CreatedAt.In(location).Format("2006-01-02")
		if expense.OccurredAt != nil {
			key = expense.OccurredAt.In(location).Format("2006-01-02")
		}
		if key >= lower && key <= window.closingKey {
			return 1, nil
		}
		return 0, nil
	}
	startKey := expense.CreatedAt.In(location).Format("2006-01-02")
	if expense.StartsAt != nil {
		startKey = expense.StartsAt.UTC().Format("2006-01-02")
	}
	hi := window.closingKey
	if expense.EndsAt != nil {
		endKey := expense.EndsAt.UTC().Format("2006-01-02")
		if endKey < hi {
			hi = endKey
		}
	}
	if expense.ArchivedAt != nil {
		archivedKey := expense.ArchivedAt.In(location).Format("2006-01-02")
		if archivedKey < hi {
			hi = archivedKey
		}
	}
	if lower > hi {
		return 0, nil
	}
	if expense.Frequency == "semanal" {
		return cardReadCountWeeklyOccurrences(startKey, lower, hi)
	}
	return cardReadCountMonthlyOccurrences(startKey, lower, hi, expense.DueDay)
}

func cardReadLateOneOffs(expenses []cardReadExpense, cardID, closedThrough string, processedAt time.Time, location *time.Location) (money, error) {
	var total money
	for _, expense := range expenses {
		if expense.IsInvoice || expense.Frequency != "unica" || !expense.CreatedAt.After(processedAt) {
			continue
		}
		key := expense.CreatedAt.In(location).Format("2006-01-02")
		if expense.OccurredAt != nil {
			key = expense.OccurredAt.In(location).Format("2006-01-02")
		}
		if key > closedThrough {
			continue
		}
		share, err := cardReadExpenseShare(expense, cardID)
		if err != nil {
			return 0, err
		}
		total, err = addCardReadMoney(total, share)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func cardReadLowerCycleKey(window cardReadCycleWindow) (string, error) {
	if window.includePeriodStart {
		return window.periodStartKey, nil
	}
	return cardReadAddDays(window.periodStartKey, 1)
}

func cardReadNextClosingOnOrAfter(closingDay int32, afterKey, notBeforeKey string) (string, error) {
	if closingDay < 1 || closingDay > 31 {
		return "", errors.New("invalid card closing day")
	}
	start, err := cardReadParseDayKey(afterKey)
	if err != nil {
		return "", err
	}
	notBefore, err := cardReadParseDayKey(notBeforeKey)
	if err != nil {
		return "", err
	}
	month := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	for index := 0; index < 1200; index++ {
		candidate := cardReadClosingDayKey(month.Year(), month.Month(), closingDay)
		if candidate > afterKey && candidate >= notBefore.Format("2006-01-02") {
			return candidate, nil
		}
		month = month.AddDate(0, 1, 0)
	}
	return "", errors.New("no card closing date found")
}

func cardReadClosingDayKey(year int, month time.Month, closingDay int32) string {
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
	day := int(closingDay)
	if day > lastDay {
		day = lastDay
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func cardReadMaxChargesForWindow(window cardReadCycleWindow) (cardReadMaxCharges, error) {
	lo, err := cardReadLowerCycleKey(window)
	if err != nil {
		return cardReadMaxCharges{}, err
	}
	maxMonthly := 0
	maxWeekly := 0
	if lo <= window.closingKey {
		loDate, err := cardReadParseDayKey(lo)
		if err != nil {
			return cardReadMaxCharges{}, err
		}
		for day := int32(1); day <= 31; day++ {
			anchor := cardReadClosingDayKey(loDate.Year()-1, time.January, day)
			count, err := cardReadCountMonthlyOccurrences(anchor, lo, window.closingKey, nil)
			if err != nil {
				return cardReadMaxCharges{}, err
			}
			if int(count) > maxMonthly {
				maxMonthly = int(count)
			}
		}
		for offset := 0; offset < 7; offset++ {
			anchor, err := cardReadAddDays(lo, -offset)
			if err != nil {
				return cardReadMaxCharges{}, err
			}
			count, err := cardReadCountWeeklyOccurrences(anchor, lo, window.closingKey)
			if err != nil {
				return cardReadMaxCharges{}, err
			}
			if int(count) > maxWeekly {
				maxWeekly = int(count)
			}
		}
	}
	if maxMonthly < 1 {
		maxMonthly = 1
	}
	if maxWeekly < 5 {
		maxWeekly = 5
	}
	return cardReadMaxCharges{Unica: 1, Mensal: maxMonthly, Semanal: maxWeekly}, nil
}

func cardReadCountWeeklyOccurrences(anchorKey, loKey, hiKey string) (int64, error) {
	anchor, err := cardReadParseDayKey(anchorKey)
	if err != nil {
		return 0, err
	}
	lo, err := cardReadParseDayKey(loKey)
	if err != nil {
		return 0, err
	}
	hi, err := cardReadParseDayKey(hiKey)
	if err != nil {
		return 0, err
	}
	first := lo
	if anchor.After(first) {
		first = anchor
	}
	if first.After(hi) {
		return 0, nil
	}
	shift := (int(anchor.Weekday()) - int(first.Weekday()) + 7) % 7
	firstOccurrence := first.AddDate(0, 0, shift)
	if firstOccurrence.After(hi) {
		return 0, nil
	}
	return int64(hi.Sub(firstOccurrence)/(7*24*time.Hour)) + 1, nil
}

func cardReadCountMonthlyOccurrences(anchorKey, loKey, hiKey string, dueDay *int32) (int64, error) {
	anchor, err := cardReadParseDayKey(anchorKey)
	if err != nil {
		return 0, err
	}
	lo, err := cardReadParseDayKey(loKey)
	if err != nil {
		return 0, err
	}
	hi, err := cardReadParseDayKey(hiKey)
	if err != nil {
		return 0, err
	}
	day := anchor.Day()
	if dueDay != nil && *dueDay >= 1 && *dueDay <= 31 {
		day = int(*dueDay)
	}
	effectiveLo := lo
	if anchor.After(effectiveLo) {
		effectiveLo = anchor
	}
	if effectiveLo.After(hi) {
		return 0, nil
	}
	month := time.Date(effectiveLo.Year(), effectiveLo.Month(), 1, 0, 0, 0, 0, time.UTC)
	var count int64
	for index := 0; index < 1200; index++ {
		lastDay := time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		clampedDay := day
		if clampedDay > lastDay {
			clampedDay = lastDay
		}
		occurrence := time.Date(month.Year(), month.Month(), clampedDay, 0, 0, 0, 0, time.UTC)
		if occurrence.After(hi) {
			return count, nil
		}
		if !occurrence.Before(effectiveLo) {
			count++
		}
		month = month.AddDate(0, 1, 0)
	}
	return 0, errors.New("card cycle exceeds the 1200 month safety bound")
}

func cardReadMonthEnd(month string) (string, error) {
	if len(month) != 7 || month[4] != '-' || !decimalDigits(month[:4]) || !decimalDigits(month[5:]) {
		return "", errors.New("invalid card obligation month")
	}
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		return "", err
	}
	return time.Date(parsed.Year(), parsed.Month()+1, 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), nil
}

func cardReadParseDayKey(value string) (time.Time, error) {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return time.Time{}, fmt.Errorf("invalid card cycle day key %q", value)
	}
	return parsed.UTC(), nil
}

func cardReadAddDays(key string, days int) (string, error) {
	parsed, err := cardReadParseDayKey(key)
	if err != nil {
		return "", err
	}
	return parsed.AddDate(0, 0, days).Format("2006-01-02"), nil
}

func addCardReadMoney(left, right money) (money, error) {
	if (right > 0 && int64(left) > cardReadMaxInt64-int64(right)) ||
		(right < 0 && int64(left) < cardReadMinInt64-int64(right)) {
		return 0, errors.New("card commitment exceeds supported money range")
	}
	return left + right, nil
}

func subtractCardReadMoney(left, right money) (money, error) {
	if (right > 0 && int64(left) < cardReadMinInt64+int64(right)) ||
		(right < 0 && int64(left) > cardReadMaxInt64+int64(right)) {
		return 0, errors.New("card commitment exceeds supported money range")
	}
	return left - right, nil
}

func multiplyCardReadMoney(value money, count int64) (money, error) {
	if count < 0 {
		return 0, errors.New("negative card occurrence count")
	}
	if count == 0 || value == 0 {
		return 0, nil
	}
	if value > 0 && int64(value) > cardReadMaxInt64/count {
		return 0, errors.New("card commitment exceeds supported money range")
	}
	if value < 0 && int64(value) < cardReadMinInt64/count {
		return 0, errors.New("card commitment exceeds supported money range")
	}
	return money(int64(value) * count), nil
}

const (
	cardReadMaxInt64 int64 = 1<<63 - 1
	cardReadMinInt64 int64 = -1 << 63
)

const cardReadListQuery = `
	SELECT
		c."id",
		c."name",
		c."limit"::text,
		c."closingDay",
		c."pendingClosingDay",
		to_char(c."archivedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(c."expiresAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(c."lastInvoicedOn", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(c."lastBillingProcessedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(c."minimumNextClosingOn", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(c."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(c."updatedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
	FROM "Card" AS c
	WHERE c."userId" = $1 AND c."archivedAt" IS NULL
	ORDER BY c."createdAt" DESC`

const cardReadCommitmentsQuery = `
	SELECT
		e."amount"::text,
		e."cardId",
		e."isInvoice",
		e."frequency",
		e."dueDay",
		to_char(e."startsAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."endsAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."occurredAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."archivedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(e."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		COALESCE((
			SELECT json_agg(json_build_object(
				'kind', split."kind",
				'cardId', split."cardId",
				'amount', split."amount"::text
			) ORDER BY split."kind", split."id")
			FROM "ExpenseSplit" AS split
			WHERE split."expenseId" = e."id"
		), '[]'::json)::text,
		COALESCE((
			SELECT json_agg(json_build_object(
				'sequence', version."sequence",
				'effectiveMonth', version."effectiveMonth",
				'amount', version."amount"::text,
				'cardId', version."cardId",
				'splits', version."splits"
			) ORDER BY version."sequence" ASC)
			FROM "ExpenseObligationVersion" AS version
			WHERE version."expenseId" = e."id"
		), '[]'::json)::text
	FROM "Expense" AS e
	WHERE e."userId" = $1
		AND e."isInvoice" = false
		AND e."systemOrigin" = 'manual'`
