package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type patrimonySettings struct {
	BaseDate       string `json:"baseDate"`
	OpeningCashBrl string `json:"openingCashBrl"`
}

type patrimonySettingsReader interface {
	ReadPatrimonySettings(context.Context, string) (*patrimonySettings, error)
}

type postgresPatrimonySettingsRepository struct {
	database *postgresDatabase
}

func (repository postgresPatrimonySettingsRepository) ReadPatrimonySettings(ctx context.Context, userID string) (*patrimonySettings, error) {
	if repository.database == nil {
		return nil, errors.New("database is not configured")
	}
	queryContext, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	pool, err := repository.database.pool()
	if err != nil {
		return nil, err
	}
	return readPatrimonySettings(queryContext, pool, userID)
}

type patrimonySettingsQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readPatrimonySettings(ctx context.Context, queryer patrimonySettingsQueryer, userID string) (*patrimonySettings, error) {
	var settings patrimonySettings
	var openingCashBrl string
	err := queryer.QueryRow(ctx, `
		SELECT to_char("baseDate", 'YYYY-MM-DD'), "openingCashBrl"::text
		FROM "PatrimonySettings"
		WHERE "userId" = $1`, userID).Scan(&settings.BaseDate, &openingCashBrl)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	settings.OpeningCashBrl = prismaDecimalString(openingCashBrl)
	return &settings, nil
}

func prismaDecimalString(value string) string {
	if !strings.Contains(value, ".") {
		return value
	}
	value = strings.TrimRight(value, "0")
	return strings.TrimRight(value, ".")
}

func registerPatrimonySettings(
	mux *http.ServeMux,
	jwtKey []byte,
	sessionReader userSessionVersionRepository,
	settingsReader patrimonySettingsReader,
) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := financialReadUserOrRespond(w, r, jwtKey, sessionReader)
		if !ok {
			return
		}
		if settingsReader == nil {
			writeInternalError(w, r)
			return
		}
		settings, err := settingsReader.ReadPatrimonySettings(r.Context(), userID)
		if err != nil {
			writeInternalError(w, r)
			return
		}
		writeJSON(w, r, http.StatusOK, settings)
	}
	mux.HandleFunc("GET /patrimony/settings", handler)
	mux.HandleFunc("GET /patrimony/settings/{$}", handler)
}

var _ patrimonySettingsQueryer = (*pgxpool.Pool)(nil)
