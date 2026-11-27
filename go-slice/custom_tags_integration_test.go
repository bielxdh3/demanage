//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPostgresCustomTagsIntegration(t *testing.T) {
	databaseURL := requireGitHubPostgresDSN(t)
	database := newPostgresDatabase(databaseURL)
	defer database.Close()

	setupContext, cancelSetup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelSetup()
	pool, err := database.pool()
	if err != nil {
		t.Fatalf("create PostgreSQL pool: %v", err)
	}
	schema, err := os.ReadFile("testdata/custom-tags.integration.sql")
	if err != nil {
		t.Fatalf("read route fixture schema: %v", err)
	}
	for _, statement := range strings.Split(string(schema), ";") {
		if statement = strings.TrimSpace(statement); statement == "" {
			continue
		}
		if _, err := pool.Exec(setupContext, statement); err != nil {
			t.Fatalf("apply route fixture schema: %v", err)
		}
	}
	_, err = pool.Exec(setupContext, `
		INSERT INTO "User" ("id", "sessionVersion") VALUES
			('synthetic-owner-a', 0),
			('synthetic-owner-b', 0)`)
	if err != nil {
		t.Fatalf("insert synthetic users: %v", err)
	}
	_, err = pool.Exec(setupContext, `
		INSERT INTO "CustomTag" ("id", "userId", "scope", "name", "color", "createdAt", "updatedAt") VALUES
			('tag-a-zulu', 'synthetic-owner-a', 'expense', 'Zulu', '#111111', '2026-02-03T04:05:06.007Z', '2026-02-03T04:05:06.007Z'),
			('tag-a-alpha', 'synthetic-owner-a', 'expense', 'Alpha', '#222222', '2026-02-04T05:06:07.089Z', '2026-02-04T05:06:07.089Z'),
			('tag-a-income', 'synthetic-owner-a', 'income', 'Income only', '#333333', '2026-02-05T06:07:08.010Z', '2026-02-05T06:07:08.010Z'),
			('tag-b-alpha', 'synthetic-owner-b', 'expense', 'A foreign tag', '#444444', '2026-02-06T07:08:09.011Z', '2026-02-06T07:08:09.011Z')`)
	if err != nil {
		t.Fatalf("insert synthetic rows: %v", err)
	}

	repository := postgresCustomTagRepository{database: database}
	handler := newHandler(database.Probe, handlerOptions{
		jwtKey:    testJWTKey,
		tagReader: repository,
	})
	ready := httptest.NewRecorder()
	handler.ServeHTTP(ready, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if ready.Code != http.StatusOK || ready.Body.String() != `{"status":"ok","service":"demanage-backend","db":"ready"}` {
		t.Fatalf("/ready = %d %s", ready.Code, ready.Body.String())
	}

	request := func(scope, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/custom-tags?scope="+url.QueryEscape(scope), nil)
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	ownerAToken := integrationToken(t, 0)
	expenses := request("expense", ownerAToken)
	if expenses.Code != http.StatusOK {
		t.Fatalf("expense tags = %d %s", expenses.Code, expenses.Body.String())
	}
	var expenseTags []customTag
	if err := json.Unmarshal(expenses.Body.Bytes(), &expenseTags); err != nil {
		t.Fatalf("decode expense tags: %v", err)
	}
	if len(expenseTags) != 2 || expenseTags[0].ID != "tag-a-alpha" || expenseTags[1].ID != "tag-a-zulu" {
		t.Fatalf("expense ordering/owner isolation = %#v", expenseTags)
	}
	for _, tag := range expenseTags {
		if tag.UserID != "synthetic-owner-a" || tag.Scope != "expense" {
			t.Fatalf("unexpected owner or scope in response: %#v", tag)
		}
	}
	if expenseTags[0].CreatedAt != "2026-02-04T05:06:07.089Z" {
		t.Fatalf("createdAt = %q, want millisecond UTC value", expenseTags[0].CreatedAt)
	}

	incomes := request("income", ownerAToken)
	var incomeTags []customTag
	if incomes.Code != http.StatusOK || json.Unmarshal(incomes.Body.Bytes(), &incomeTags) != nil || len(incomeTags) != 1 || incomeTags[0].ID != "tag-a-income" {
		t.Fatalf("income scope filtering = %d %s %#v", incomes.Code, incomes.Body.String(), incomeTags)
	}

	if _, err := pool.Exec(setupContext, `UPDATE "User" SET "sessionVersion" = 1 WHERE "id" = 'synthetic-owner-a'`); err != nil {
		t.Fatalf("invalidate synthetic session: %v", err)
	}
	invalidated := request("expense", ownerAToken)
	var invalidatedBody struct {
		Error string `json:"error"`
	}
	if invalidated.Code != http.StatusUnauthorized || json.Unmarshal(invalidated.Body.Bytes(), &invalidatedBody) != nil || invalidatedBody.Error != "N\u00e3o autenticado" {
		t.Fatalf("invalidated session = %d %s", invalidated.Code, invalidated.Body.String())
	}

	if _, err := pool.Exec(setupContext, `DROP TABLE "CustomTag"`); err != nil {
		t.Fatalf("prepare synthetic query failure: %v", err)
	}
	databaseError := request("expense", integrationToken(t, 1))
	if databaseError.Code != http.StatusInternalServerError || databaseError.Body.String() != `{"error":"Internal server error"}` || strings.Contains(databaseError.Body.String(), "CustomTag") {
		t.Fatalf("database failure leaked details: %d %s", databaseError.Code, databaseError.Body.String())
	}
}

func requireGitHubPostgresDSN(t *testing.T) string {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("PostgreSQL integration test must run on GitHub Actions")
	}
	databaseURL := os.Getenv("GO_TEST_DATABASE_URL")
	if databaseURL == "" || databaseURL == os.Getenv("DATABASE_URL") {
		t.Fatal("GO_TEST_DATABASE_URL must be set and distinct from DATABASE_URL")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() != "localhost" || parsed.Port() != "5432" || strings.TrimPrefix(parsed.Path, "/") != "demanage_go_test" || parsed.User == nil || parsed.User.Username() != "demanage" {
		t.Fatal("GO_TEST_DATABASE_URL must target the fixed GitHub PostgreSQL service database")
	}
	password, ok := parsed.User.Password()
	if !ok || password != "demanage" {
		t.Fatal("GO_TEST_DATABASE_URL must use the synthetic service password")
	}
	return databaseURL
}

func integrationToken(t *testing.T, sessionVersion int) string {
	t.Helper()
	return signTestToken(t, jwt.SigningMethodHS256, map[string]any{
		"userId":         "synthetic-owner-a",
		"sessionVersion": sessionVersion,
		"exp":            time.Now().Add(time.Hour).Unix(),
	})
}
