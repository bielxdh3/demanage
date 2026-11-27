package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type profileRead struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Email            string       `json:"email"`
	HasRecoveryCode  bool         `json:"hasRecoveryCode"`
	Salary           float64      `json:"salary"`
	SalaryReceiveDay *int32       `json:"salaryReceiveDay"`
	Notes            *string      `json:"notes"`
	CreatedAt        string       `json:"createdAt"`
	UpdatedAt        string       `json:"updatedAt"`
}

type profileReadRepository interface {
	ReadProfile(context.Context, string) (*profileRead, error)
}

type postgresProfileReadRepository struct {
	database *postgresDatabase
}

func (repository postgresProfileReadRepository) ReadProfile(ctx context.Context, userID string) (*profileRead, error) {
	if repository.database == nil {
		return nil, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, err
	}
	return readProfile(queryContext, pool, userID)
}

type profileReadQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readProfile(ctx context.Context, queryer profileReadQueryer, userID string) (*profileRead, error) {
	var profile profileRead
	var salary, notes pgtype.Text
	var salaryReceiveDay pgtype.Int4
	var createdAt, updatedAt time.Time
	err := queryer.QueryRow(ctx, `
		SELECT u.id, u.name, u.email,
			COALESCE(NULLIF(u."recoveryCodeHash", ''), '') <> '', u.salary::text,
			(
				SELECT e."receiveDay"
				FROM "Entry" e
				WHERE e."userId" = u.id
					AND e."type" = 'salario'
					AND e."frequency" = 'mensal'
					AND e.name = 'Salário'
				ORDER BY e."createdAt" ASC, e.id ASC
				LIMIT 1
			),
			u.notes, u."createdAt", u."updatedAt"
		FROM "User" u
		WHERE u.id = $1`, userID).Scan(
		&profile.ID,
		&profile.Name,
		&profile.Email,
		&profile.HasRecoveryCode,
		&salary,
		&salaryReceiveDay,
		&notes,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if salary.Valid {
		value, err := strconv.ParseFloat(salary.String, 64)
		if err != nil {
			return nil, errors.New("invalid profile salary")
		}
		profile.Salary = value
	}
	if salaryReceiveDay.Valid {
		profile.SalaryReceiveDay = &salaryReceiveDay.Int32
	}
	if notes.Valid {
		profile.Notes = &notes.String
	}
	profile.CreatedAt = createdAt.UTC().Format("2006-01-02T15:04:05.000Z")
	profile.UpdatedAt = updatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	return &profile, nil
}

func registerProfileRead(
	mux *http.ServeMux,
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
	profileReader profileReadRepository,
) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		if profileReader == nil {
			writeInternalError(w, r)
			return
		}
		profile, err := profileReader.ReadProfile(r.Context(), userID)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		if profile == nil || profile.ID != userID {
			writeJSON(w, r, http.StatusUnauthorized, struct {
				Error string `json:"error"`
			}{"N\u00e3o autenticado"})
			return
		}
		writeJSON(w, r, http.StatusOK, struct {
			User *profileRead `json:"user"`
		}{profile})
	}
	mux.HandleFunc("GET /auth/me", handler)
	mux.HandleFunc("GET /auth/me/{$}", handler)
}

var _ profileReadQueryer = (*pgxpool.Pool)(nil)
