package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

type money int64

type percentage int32

type expenseCustomTag struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type expenseSplit struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	CardID   *string    `json:"cardId"`
	Percent  percentage `json:"percent"`
	Amount   money      `json:"amount"`
	CardName *string    `json:"cardName"`
}

type expensePayment struct {
	ID        string `json:"id"`
	ExpenseID string `json:"expenseId"`
	Month     string `json:"month"`
	Amount    money  `json:"amount"`
	PaidAt    string `json:"paidAt"`
	CreatedAt string `json:"createdAt"`
}

type expense struct {
	ID                 string             `json:"id"`
	UserID             string             `json:"userId"`
	CardID             *string            `json:"cardId"`
	CustomTagID        *string            `json:"customTagId"`
	Name               string             `json:"name"`
	Amount             money              `json:"amount"`
	Category           string             `json:"category"`
	Frequency          string             `json:"frequency"`
	DueDay             *int32             `json:"dueDay"`
	StartsAt           *string            `json:"startsAt"`
	EndsAt             *string            `json:"endsAt"`
	OccurredAt         *string            `json:"occurredAt"`
	PaidForMonth       *string            `json:"paidForMonth"`
	PaidAt             *string            `json:"paidAt"`
	Notes              *string            `json:"notes"`
	IsInvoice          bool               `json:"isInvoice"`
	BillingPeriodStart *string            `json:"billingPeriodStart"`
	BillingPeriodEnd   *string            `json:"billingPeriodEnd"`
	ArchivedAt         *string            `json:"archivedAt"`
	VoidedAt           *string            `json:"voidedAt"`
	SystemOrigin       string             `json:"systemOrigin"`
	CreatedAt          string             `json:"createdAt"`
	UpdatedAt          string             `json:"updatedAt"`
	CustomTag          *expenseCustomTag `json:"customTag"`
	Splits             []expenseSplit    `json:"splits"`
	Payments           []expensePayment  `json:"payments"`
}

func (value money) MarshalJSON() ([]byte, error) {
	return marshalScaled(int64(value), 100), nil
}

func (value *money) UnmarshalJSON(data []byte) error {
	parsed, err := parseScaled(data, 2)
	if err != nil {
		return err
	}
	*value = money(parsed)
	return nil
}

func (value percentage) MarshalJSON() ([]byte, error) {
	return marshalScaled(int64(value), 100), nil
}

func (value *percentage) UnmarshalJSON(data []byte) error {
	parsed, err := parseScaled(data, 2)
	if err != nil {
		return err
	}
	if parsed < -2147483648 || parsed > 2147483647 {
		return errors.New("percentage is out of range")
	}
	*value = percentage(parsed)
	return nil
}

func marshalScaled(value int64, scale uint64) []byte {
	negative := value < 0
	magnitude := uint64(value)
	if negative {
		magnitude = uint64(-(value + 1)) + 1
	}
	result := make([]byte, 0, 24)
	if negative {
		result = append(result, '-')
	}
	result = strconv.AppendUint(result, magnitude/scale, 10)
	result = append(result, '.')
	fraction := strconv.AppendUint(nil, magnitude%scale, 10)
	if len(fraction) == 1 {
		result = append(result, '0')
	}
	result = append(result, fraction...)
	return result
}

func parseScaled(data []byte, precision int) (int64, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return 0, errors.New("empty decimal value")
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return 0, err
		}
		data = []byte(value)
	}

	value := string(data)
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	if value == "" || strings.HasPrefix(value, "+") {
		return 0, errors.New("invalid decimal value")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, errors.New("invalid decimal value")
	}
	whole := parts[0]
	fraction := ""
	if whole == "" {
		whole = "0"
	}
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if !decimalDigits(whole) || (fraction != "" && !decimalDigits(fraction)) {
		return 0, errors.New("invalid decimal value")
	}
	if len(fraction) > precision {
		if strings.Trim(fraction[precision:], "0") != "" {
			return 0, errors.New("decimal precision exceeds supported scale")
		}
		fraction = fraction[:precision]
	}
	for len(fraction) < precision {
		fraction += "0"
	}
	combined := strings.TrimLeft(whole, "0") + fraction
	if combined == "" {
		combined = "0"
	}
	if negative {
		combined = "-" + combined
	}
	return strconv.ParseInt(combined, 10, 64)
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
