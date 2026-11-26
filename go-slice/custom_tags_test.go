package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testJWTKey = []byte("synthetic-only-custom-tag-test-key")

type fakeCustomTagRepository struct {
	userID          string
	sessionVersion  int64
	userExists      bool
	userError       error
	listUserID      string
	listScope       string
	tags            []customTag
	listError       error
	listCalls       int
}

func (f *fakeCustomTagRepository) FindUserSessionVersion(_ context.Context, userID string) (int64, bool, error) {
	f.userID = userID
	return f.sessionVersion, f.userExists, f.userError
}

func (f *fakeCustomTagRepository) ListCustomTags(_ context.Context, userID, scope string) ([]customTag, error) {
	f.listCalls++
	f.listUserID = userID
	f.listScope = scope
	return f.tags, f.listError
}

func TestCustomTagsContractMatchesExpress(t *testing.T) {
	repository := &fakeCustomTagRepository{
		userExists: true,
		tags: []customTag{{
			ID: "tag-1", UserID: "owner-1", Scope: "expense", Name: "Alimentação", Color: "#AABBCC",
			CreatedAt: "2026-02-03T04:05:06.007Z", UpdatedAt: "2026-02-04T05:06:07.089Z",
		}},
	}
	response := serveCustomTags(t, repository, map[string]any{"userId": "owner-1", "sessionVersion": 0, "exp": 4102444800}, "scope=expense")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body.String())
	}
	if got, want := response.Body.String(), `[{"id":"tag-1","userId":"owner-1","scope":"expense","name":"Alimentação","color":"#AABBCC","createdAt":"2026-02-03T04:05:06.007Z","updatedAt":"2026-02-04T05:06:07.089Z"}]`; got != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
	if repository.userID != "owner-1" || repository.listUserID != "owner-1" || repository.listScope != "expense" {
		t.Fatalf("database filters user=%q list user=%q scope=%q", repository.userID, repository.listUserID, repository.listScope)
	}
	assertExpressJSONHeaders(t, response)
}

func TestCustomTagsRetainsDuplicateNamesReturnedByDatabase(t *testing.T) {
	createdAt := "2026-03-04T05:06:07.000Z"
	repository := &fakeCustomTagRepository{
		userExists: true,
		tags: []customTag{
			{ID: "tag-1", UserID: "owner-1", Scope: "income", Name: "Extra", Color: "#111111", CreatedAt: createdAt, UpdatedAt: createdAt},
			{ID: "tag-2", UserID: "owner-1", Scope: "income", Name: "Extra", Color: "#222222", CreatedAt: createdAt, UpdatedAt: createdAt},
		},
	}
	response := serveCustomTags(t, repository, map[string]any{"userId": "owner-1", "sessionVersion": 0, "exp": 4102444800}, "scope=income")
	want := `[{"id":"tag-1","userId":"owner-1","scope":"income","name":"Extra","color":"#111111","createdAt":"2026-03-04T05:06:07.000Z","updatedAt":"2026-03-04T05:06:07.000Z"},{"id":"tag-2","userId":"owner-1","scope":"income","name":"Extra","color":"#222222","createdAt":"2026-03-04T05:06:07.000Z","updatedAt":"2026-03-04T05:06:07.000Z"}]`
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if repository.listCalls != 1 {
		t.Fatalf("database list calls = %d, want 1", repository.listCalls)
	}
}

func TestCustomTagsRejectsInvalidAndDuplicateScopesLikeExpress(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantError  string
	}{
		{name: "missing", wantError: "Query obrigatória: scope=expense|income"},
		{name: "invalid", query: "scope=Expense", wantError: "Query obrigatória: scope=expense|income"},
		{name: "duplicate same", query: "scope=expense&scope=expense", wantError: "Parâmetro inválido: scope"},
		{name: "duplicate different", query: "scope=expense&scope=income", wantError: "Parâmetro inválido: scope"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeCustomTagRepository{userExists: true}
			response := serveCustomTags(t, repository, map[string]any{"userId": "owner-1", "sessionVersion": 0, "exp": 4102444800}, test.query)
			if response.Code != http.StatusBadRequest || response.Body.String() != `{"error":"`+test.wantError+`"}` {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if repository.listCalls != 0 {
				t.Fatalf("invalid scope reached tag query %d times", repository.listCalls)
			}
		})
	}
}

func TestCustomTagsAuthenticationAndSessionParity(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name           string
		claims         map[string]any
		sessionVersion int64
		exists         bool
		wantStatus     int
	}{
		{name: "valid explicit session", claims: map[string]any{"userId": "owner-1", "sessionVersion": 3, "exp": now.Add(time.Hour).Unix()}, sessionVersion: 3, exists: true, wantStatus: http.StatusOK},
		{name: "omitted session defaults to zero", claims: map[string]any{"userId": "owner-1", "exp": now.Add(time.Hour).Unix()}, exists: true, wantStatus: http.StatusOK},
		{name: "omitted session does not match rotated user", claims: map[string]any{"userId": "owner-1", "exp": now.Add(time.Hour).Unix()}, sessionVersion: 1, exists: true, wantStatus: http.StatusUnauthorized},
		{name: "revoked session", claims: map[string]any{"userId": "owner-1", "sessionVersion": 2, "exp": now.Add(time.Hour).Unix()}, sessionVersion: 3, exists: true, wantStatus: http.StatusUnauthorized},
		{name: "deleted user", claims: map[string]any{"userId": "owner-1", "sessionVersion": 0, "exp": now.Add(time.Hour).Unix()}, exists: false, wantStatus: http.StatusUnauthorized},
		{name: "expired token", claims: map[string]any{"userId": "owner-1", "sessionVersion": 0, "exp": now.Add(-time.Minute).Unix()}, exists: true, wantStatus: http.StatusUnauthorized},
		{name: "session field casing follows JavaScript exact property", claims: map[string]any{"userId": "owner-1", "SessionVersion": 99, "exp": now.Add(time.Hour).Unix()}, exists: true, wantStatus: http.StatusOK},
		{name: "invalid session version type", claims: map[string]any{"userId": "owner-1", "sessionVersion": "0", "exp": now.Add(time.Hour).Unix()}, exists: true, wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &fakeCustomTagRepository{userExists: test.exists, sessionVersion: test.sessionVersion}
			response := serveCustomTags(t, repository, test.claims, "scope=expense")
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantStatus == http.StatusUnauthorized && response.Body.String() != `{"error":"Não autenticado"}` {
				t.Fatalf("unauthorized body = %s", response.Body.String())
			}
		})
	}
}

func TestCustomTagsRejectsMissingAndNonHS256Cookies(t *testing.T) {
	repository := &fakeCustomTagRepository{userExists: true}
	missing := httptest.NewRecorder()
	newHandler(func(context.Context) error { return nil }, handlerOptions{jwtKey: testJWTKey, tagReader: repository}).ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/custom-tags?scope=expense", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing cookie status = %d, want 401", missing.Code)
	}
	unauthorizedInvalidScope := httptest.NewRecorder()
	newHandler(func(context.Context) error { return nil }, handlerOptions{jwtKey: testJWTKey, tagReader: repository}).ServeHTTP(unauthorizedInvalidScope, httptest.NewRequest(http.MethodGet, "/custom-tags?scope=invalid", nil))
	if unauthorizedInvalidScope.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated invalid-scope status = %d, want 401", unauthorizedInvalidScope.Code)
	}

	token := signTestToken(t, jwt.SigningMethodHS512, map[string]any{"userId": "owner-1", "sessionVersion": 0, "exp": time.Now().Add(time.Hour).Unix()})
	request := httptest.NewRequest(http.MethodGet, "/custom-tags?scope=expense", nil)
	request.AddCookie(&http.Cookie{Name: "demanage-token", Value: token})
	response := httptest.NewRecorder()
	newHandler(func(context.Context) error { return nil }, handlerOptions{jwtKey: testJWTKey, tagReader: repository}).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("HS512 status = %d, want 401", response.Code)
	}
}

func TestCustomTagsRejectsMalformedAndWrongKeyTokens(t *testing.T) {
	for name, token := range map[string]string{
		"malformed": "not.a.jwt",
		"wrong key": mustSignToken(t, []byte("different synthetic test key")),
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/custom-tags?scope=expense", nil)
			request.AddCookie(&http.Cookie{Name: "demanage-token", Value: token})
			response := httptest.NewRecorder()
			newHandler(func(context.Context) error { return nil }, handlerOptions{jwtKey: testJWTKey, tagReader: &fakeCustomTagRepository{userExists: true}}).ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", response.Code)
			}
		})
	}
}

func TestCustomTagsSanitizesDatabaseFailuresAsServerErrors(t *testing.T) {
	for name, repository := range map[string]*fakeCustomTagRepository{
		"session lookup": {userError: errors.New("private database host and password")},
		"tag query":     {userExists: true, listError: errors.New("private database host and password")},
	} {
		t.Run(name, func(t *testing.T) {
			response := serveCustomTags(t, repository, map[string]any{"userId": "owner-1", "sessionVersion": 0, "exp": 4102444800}, "scope=expense")
			if response.Code != http.StatusInternalServerError || response.Body.String() != `{"error":"Internal server error"}` {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}

func serveCustomTags(t testing.TB, repository *fakeCustomTagRepository, claims map[string]any, query string) *httptest.ResponseRecorder {
	token := signTestToken(t, jwt.SigningMethodHS256, claims)
	request := httptest.NewRequest(http.MethodGet, "/custom-tags?"+query, nil)
	request.AddCookie(&http.Cookie{Name: "demanage-token", Value: token})
	response := httptest.NewRecorder()
	newHandler(func(context.Context) error { return nil }, handlerOptions{jwtKey: testJWTKey, tagReader: repository}).ServeHTTP(response, request)
	return response
}

func signTestToken(t testing.TB, method jwt.SigningMethod, claims map[string]any) string {
	t.Helper()
	token := jwt.NewWithClaims(method, jwt.MapClaims(claims))
	signed, err := token.SignedString(testJWTKey)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func mustSignToken(t testing.TB, key []byte) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"userId": "owner-1", "sessionVersion": 0, "exp": 4102444800})
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}
