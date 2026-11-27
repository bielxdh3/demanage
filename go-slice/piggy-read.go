package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"
	_ "time/tzdata"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type piggyCDIPercent int64

func (value piggyCDIPercent) MarshalJSON() ([]byte, error) {
	negative := value < 0
	magnitude := uint64(value)
	if negative {
		magnitude = uint64(-(value + 1)) + 1
	}
	result := make([]byte, 0, 16)
	if negative {
		result = append(result, '-')
	}
	result = strconv.AppendUint(result, magnitude/10000, 10)
	result = append(result, '.')
	fraction := strconv.AppendUint(nil, magnitude%10000, 10)
	for padding := 4 - len(fraction); padding > 0; padding-- {
		result = append(result, '0')
	}
	result = append(result, fraction...)
	return result, nil
}

type piggyBankResponse struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	GoalAmount             *money          `json:"goalAmount"`
	TargetDate             *string         `json:"targetDate"`
	MonthlyGoal            money           `json:"monthlyGoal"`
	AutoDebitPaidThisMonth bool            `json:"autoDebitPaidThisMonth"`
	AutoDebit              bool            `json:"autoDebit"`
	AutoDebitDay           int32           `json:"autoDebitDay"`
	IsEmergency            bool            `json:"isEmergency"`
	YieldEnabled           bool            `json:"yieldEnabled"`
	CDIPercent             piggyCDIPercent `json:"cdiPercent"`
	InterestAccruedThrough *string         `json:"interestAccruedThrough"`
	ArchivedAt             *string         `json:"archivedAt"`
	CompletedAt            *string         `json:"completedAt"`
	Balance                money           `json:"balance"`
	Progress               float64         `json:"progress"`
	Remaining              money           `json:"remaining"`
	CreatedAt              string          `json:"createdAt"`
	UpdatedAt              string          `json:"updatedAt"`
}

type piggyTransactionRecord struct {
	typeName string
	source   string
	amount   money
	date     string
}

type piggyBankRecord struct {
	id                     string
	name                   string
	goalAmount             *money
	targetDate             *string
	monthlyGoal            money
	autoDebit              bool
	autoDebitDay           int32
	isEmergency            bool
	yieldEnabled           bool
	cdiPercent             piggyCDIPercent
	interestAccruedThrough *string
	archivedAt             *string
	completedAt            *string
	createdAt              string
	updatedAt              string
	transactions           []piggyTransactionRecord
}

type piggyBankListRepository interface {
	ListPiggyBanks(context.Context, string, bool) ([]piggyBankResponse, error)
}

type piggyBankQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type postgresPiggyBankRepository struct {
	database *postgresDatabase
	now      func() time.Time
}

func piggyMonthInSaoPaulo(now time.Time) (string, error) {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return "", err
	}
	return now.In(location).Format("2006-01"), nil
}

func (repository postgresPiggyBankRepository) ListPiggyBanks(
	ctx context.Context,
	userID string,
	includeArchived bool,
) ([]piggyBankResponse, error) {
	if repository.database == nil {
		return nil, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, err
	}
	now := time.Now
	if repository.now != nil {
		now = repository.now
	}
	return queryPiggyBanks(queryContext, pool, userID, includeArchived, now())
}

func newPiggyBanksHandler(
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
	bankReader piggyBankListRepository,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		values, exists := r.URL.Query()["includeArchived"]
		if exists && len(values) != 1 {
			writeJSON(w, r, http.StatusBadRequest, struct {
				Error string `json:"error"`
			}{"Parâmetro inválido: includeArchived"})
			return
		}
		if bankReader == nil {
			writeInternalError(w, r)
			return
		}
		includeArchived := exists && values[0] == "true"
		banks, err := bankReader.ListPiggyBanks(r.Context(), userID, includeArchived)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if banks == nil {
			banks = []piggyBankResponse{}
		}
		writeJSON(w, r, http.StatusOK, banks)
	}
}

func queryPiggyBanks(
	ctx context.Context,
	queryer piggyBankQueryer,
	userID string,
	includeArchived bool,
	now time.Time,
) ([]piggyBankResponse, error) {
	currentMonth, err := piggyMonthInSaoPaulo(now)
	if err != nil {
		return nil, err
	}
	rows, err := queryer.Query(ctx, piggyBankListQuery, userID, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]piggyBankRecord, 0)
	indices := make(map[string]int)
	for rows.Next() {
		var id, name, monthlyGoal, cdiPercent, createdAt, updatedAt pgtype.Text
		var goalAmount, targetDate, interestAccruedThrough pgtype.Text
		var archivedAt, completedAt pgtype.Text
		var autoDebit, isEmergency, yieldEnabled bool
		var autoDebitDay pgtype.Int4
		var transactionID, transactionType, transactionSource pgtype.Text
		var transactionAmount, transactionDate pgtype.Text
		if err := rows.Scan(
			&id,
			&name,
			&goalAmount,
			&targetDate,
			&monthlyGoal,
			&autoDebit,
			&autoDebitDay,
			&isEmergency,
			&yieldEnabled,
			&cdiPercent,
			&interestAccruedThrough,
			&archivedAt,
			&completedAt,
			&createdAt,
			&updatedAt,
			&transactionID,
			&transactionType,
			&transactionSource,
			&transactionAmount,
			&transactionDate,
		); err != nil {
			return nil, err
		}

		index, exists := indices[id.String]
		if !exists {
			monthlyValue, err := parseMoney(monthlyGoal.String)
			if err != nil {
				return nil, err
			}
			cdiValue, err := parseScaled([]byte(cdiPercent.String), 4)
			if err != nil {
				return nil, err
			}
			record := piggyBankRecord{
				id:                     id.String,
				name:                   name.String,
				monthlyGoal:            monthlyValue,
				autoDebit:              autoDebit,
				autoDebitDay:           autoDebitDay.Int32,
				isEmergency:            isEmergency,
				yieldEnabled:           yieldEnabled,
				cdiPercent:             piggyCDIPercent(cdiValue),
				interestAccruedThrough: textPointer(interestAccruedThrough),
				archivedAt:             textPointer(archivedAt),
				completedAt:            textPointer(completedAt),
				createdAt:              createdAt.String,
				updatedAt:              updatedAt.String,
				transactions:           []piggyTransactionRecord{},
			}
			if goalAmount.Valid {
				value, err := parseMoney(goalAmount.String)
				if err != nil {
					return nil, err
				}
				record.goalAmount = &value
			}
			record.targetDate = textPointer(targetDate)
			if record.goalAmount != nil && *record.goalAmount <= 0 {
				record.goalAmount = nil
			}
			index = len(records)
			indices[record.id] = index
			records = append(records, record)
		}

		if transactionID.Valid {
			if !transactionType.Valid || !transactionSource.Valid || !transactionAmount.Valid || !transactionDate.Valid {
				return nil, errors.New("incomplete piggy transaction row")
			}
			amount, err := parseMoney(transactionAmount.String)
			if err != nil {
				return nil, err
			}
			records[index].transactions = append(records[index].transactions, piggyTransactionRecord{
				typeName: transactionType.String,
				source:   transactionSource.String,
				amount:   amount,
				date:     transactionDate.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	banks := make([]piggyBankResponse, 0, len(records))
	for _, record := range records {
		banks = append(banks, serializePiggyBankRecord(record, currentMonth))
	}
	return banks, nil
}

func serializePiggyBankRecord(record piggyBankRecord, currentMonth string) piggyBankResponse {
	balance := money(0)
	autoDebitPaidThisMonth := false
	for _, transaction := range record.transactions {
		if transaction.typeName == "withdraw" {
			balance -= transaction.amount
		} else {
			balance += transaction.amount
		}
		if transaction.typeName == "deposit" && transaction.source == "auto_debit" && len(transaction.date) >= 7 && transaction.date[:7] == currentMonth {
			autoDebitPaidThisMonth = true
		}
	}

	progress := float64(0)
	remaining := money(0)
	if record.goalAmount != nil {
		if balance < *record.goalAmount {
			remaining = *record.goalAmount - balance
		}
		progress = (float64(balance) / 100) / (float64(*record.goalAmount) / 100)
		if progress > 1 {
			progress = 1
		}
	}

	return piggyBankResponse{
		ID:                     record.id,
		Name:                   record.name,
		GoalAmount:             record.goalAmount,
		TargetDate:             record.targetDate,
		MonthlyGoal:            record.monthlyGoal,
		AutoDebitPaidThisMonth: autoDebitPaidThisMonth,
		AutoDebit:              record.autoDebit,
		AutoDebitDay:           record.autoDebitDay,
		IsEmergency:            record.isEmergency,
		YieldEnabled:           record.yieldEnabled,
		CDIPercent:             record.cdiPercent,
		InterestAccruedThrough: record.interestAccruedThrough,
		ArchivedAt:             record.archivedAt,
		CompletedAt:            record.completedAt,
		Balance:                balance,
		Progress:               progress,
		Remaining:              remaining,
		CreatedAt:              record.createdAt,
		UpdatedAt:              record.updatedAt,
	}
}

const piggyBankListQuery = `
	SELECT
		bank."id",
		bank."name",
		bank."goalAmount"::text,
		to_char(bank."targetDate", 'YYYY-MM-DD'),
		bank."monthlyGoal"::text,
		bank."autoDebit",
		bank."autoDebitDay",
		bank."isEmergency",
		bank."yieldEnabled",
		bank."cdiPercent"::text,
		to_char(bank."interestAccruedThrough", 'YYYY-MM-DD'),
		to_char(bank."archivedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(bank."completedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(bank."createdAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		to_char(bank."updatedAt", 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
		piggy_tx."id",
		piggy_tx."type",
		piggy_tx."source",
		piggy_tx."amount"::text,
		to_char(piggy_tx."date", 'YYYY-MM-DD')
	FROM "PiggyBank" AS bank
	LEFT JOIN "PiggyTransaction" AS piggy_tx
		ON piggy_tx."piggyBankId" = bank."id"
		AND piggy_tx."userId" = bank."userId"
	WHERE bank."userId" = $1
		AND ($2::boolean OR bank."archivedAt" IS NULL)
	ORDER BY bank."createdAt" DESC`
